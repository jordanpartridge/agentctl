package container

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SpyOptions controls what the spy command displays.
type SpyOptions struct {
	Raw       bool // emit raw JSONL lines
	ToolsOnly bool // only show tool_use events
	Thinking  bool // include thinking blocks
	Verbose   bool // include tool results
	JSON      bool // structured JSON output for piping
}

// claudeConfig represents the top-level .claude.json file.
type claudeConfig struct {
	Projects map[string]projectEntry `json:"projects"`
}

type projectEntry struct {
	LastSessionID string `json:"lastSessionId"`
}

// jsonlMessage is the envelope for every line in the session JSONL.
type jsonlMessage struct {
	Type      string          `json:"type"`
	Message   *messageBody    `json:"message,omitempty"`
	Timestamp string          `json:"timestamp,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type messageBody struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type     string          `json:"type"`
	Name     string          `json:"name,omitempty"`
	Text     string          `json:"text,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
}

// toolInput holds the most common input fields we render.
type toolInput struct {
	Command  string `json:"command"`
	FilePath string `json:"file_path"`
	Pattern  string `json:"pattern"`
	Query    string `json:"query"`
	URL      string `json:"url"`
	Content  string `json:"content"`
}

type progressData struct {
	Type               string `json:"type"`
	ElapsedTimeSeconds int    `json:"elapsedTimeSeconds"`
	TotalLines         int    `json:"totalLines"`
	Name               string `json:"name"`
}

// Spy streams real-time session activity from a running agent's clone directory.
func Spy(name string, opts SpyOptions) error {
	agent, err := loadAgent(name)
	if err != nil {
		return fmt.Errorf("agent %q not found", name)
	}

	if _, err := os.Stat(agent.CloneDir); err != nil {
		return fmt.Errorf("clone directory %q not found — is the agent running?", agent.CloneDir)
	}

	// Discover the session JSONL file path in the local clone's .claude dir.
	sessionPath, err := discoverSessionFile(name)
	if err != nil {
		return fmt.Errorf("session discovery failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Spying on agent %s (Ctrl+C to stop)...\n", name)
	fmt.Fprintf(os.Stderr, "Session: %s\n", sessionPath)
	fmt.Fprintln(os.Stderr, "---")

	cmd := exec.Command("tail", "-f", "-n", "+1", sessionPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe failed: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("tail failed: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	// Allow up to 1 MB lines — JSONL messages can be large.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		if opts.Raw {
			fmt.Println(line)
			continue
		}

		renderLine(line, opts)
	}

	return cmd.Wait()
}

// discoverSessionFile finds the active Claude session JSONL in the agent's clone dir.
func discoverSessionFile(name string) (string, error) {
	agent, err := loadAgent(name)
	if err != nil {
		return "", fmt.Errorf("agent not found: %w", err)
	}

	claudeJSON := filepath.Join(agent.CloneDir, ".claude.json")
	out, err := os.ReadFile(claudeJSON)
	if err != nil {
		// Fall back to the most recently modified JSONL anywhere in the clone
		fallback, ferr := exec.Command("sh", "-c",
			fmt.Sprintf("find %s/.claude/projects -name '*.jsonl' 2>/dev/null | xargs ls -t 2>/dev/null | head -1", agent.CloneDir)).Output()
		if ferr == nil && len(strings.TrimSpace(string(fallback))) > 0 {
			return strings.TrimSpace(string(fallback)), nil
		}
		return "", fmt.Errorf("could not read .claude.json: %w", err)
	}

	var cfg claudeConfig
	if err := json.Unmarshal(out, &cfg); err != nil {
		return "", fmt.Errorf("could not parse .claude.json: %w", err)
	}

	var sessionID string
	for _, proj := range cfg.Projects {
		if proj.LastSessionID != "" {
			sessionID = proj.LastSessionID
			break
		}
	}
	if sessionID == "" {
		return "", fmt.Errorf("no lastSessionId found — has Claude started a session?")
	}

	projectsDir := filepath.Join(agent.CloneDir, ".claude", "projects")
	entries, _ := os.ReadDir(projectsDir)
	for _, e := range entries {
		candidate := filepath.Join(projectsDir, e.Name(), sessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("session file %s.jsonl not found", sessionID)
}

// renderLine parses a single JSONL line and emits formatted output.
func renderLine(line string, opts SpyOptions) {
	var msg jsonlMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		// Not valid JSON — print as-is with timestamp.
		fmt.Printf("%s  %s\n", ts(), line)
		return
	}

	if opts.JSON {
		renderJSON(msg, opts)
		return
	}

	switch {
	case msg.Message != nil:
		renderMessage(msg, opts)
	case msg.Type == "progress":
		renderProgress(msg, opts)
	default:
		if opts.Verbose {
			fmt.Printf("%s  [%s]\n", ts(), msg.Type)
		}
	}
}

func renderMessage(msg jsonlMessage, opts SpyOptions) {
	if msg.Message == nil {
		return
	}

	role := msg.Message.Role
	for _, block := range msg.Message.Content {
		switch block.Type {
		case "tool_use":
			renderToolUse(block, opts)
		case "text":
			if opts.ToolsOnly {
				continue
			}
			if role == "assistant" {
				text := truncate(block.Text, 120)
				fmt.Printf("%s  %s\n", ts(), text)
			}
		case "thinking":
			if !opts.Thinking {
				continue
			}
			text := truncate(block.Thinking, 100)
			fmt.Printf("%s  \033[2m[thinking] %s\033[0m\n", ts(), text)
		case "tool_result":
			if !opts.Verbose {
				continue
			}
			text := truncate(block.Text, 80)
			fmt.Printf("%s  \033[2m  -> %s\033[0m\n", ts(), text)
		}
	}
}

func renderToolUse(block contentBlock, opts SpyOptions) {
	var ti toolInput
	json.Unmarshal(block.Input, &ti)

	summary := toolSummary(block.Name, ti)
	fmt.Printf("%s  > %s: %s\n", ts(), block.Name, summary)
}

func toolSummary(name string, ti toolInput) string {
	switch name {
	case "Bash":
		return truncate(ti.Command, 100)
	case "Read":
		return ti.FilePath
	case "Write":
		return ti.FilePath
	case "Edit":
		return ti.FilePath
	case "Glob":
		return ti.Pattern
	case "Grep":
		return ti.Pattern
	case "WebFetch":
		return ti.URL
	case "WebSearch":
		return truncate(ti.Query, 80)
	case "Task":
		return truncate(ti.Content, 80)
	default:
		if ti.FilePath != "" {
			return ti.FilePath
		}
		if ti.Command != "" {
			return truncate(ti.Command, 80)
		}
		raw, _ := json.Marshal(ti)
		return truncate(string(raw), 80)
	}
}

func renderProgress(msg jsonlMessage, opts SpyOptions) {
	if opts.ToolsOnly {
		return
	}
	var pd progressData
	if err := json.Unmarshal(msg.Data, &pd); err != nil {
		return
	}

	switch pd.Type {
	case "bash_progress":
		fmt.Printf("\r%s  ... running (%ds, %d lines)", ts(), pd.ElapsedTimeSeconds, pd.TotalLines)
	case "hook_progress":
		fmt.Printf("%s  [hook] %s\n", ts(), pd.Name)
	default:
		if opts.Verbose {
			fmt.Printf("%s  [progress:%s]\n", ts(), pd.Type)
		}
	}
}

func renderJSON(msg jsonlMessage, opts SpyOptions) {
	if msg.Message == nil {
		return
	}

	for _, block := range msg.Message.Content {
		if opts.ToolsOnly && block.Type != "tool_use" {
			continue
		}
		if !opts.Thinking && block.Type == "thinking" {
			continue
		}
		if !opts.Verbose && block.Type == "tool_result" {
			continue
		}

		event := map[string]interface{}{
			"time": time.Now().Format(time.RFC3339),
			"type": block.Type,
		}
		switch block.Type {
		case "tool_use":
			event["tool"] = block.Name
			var ti toolInput
			json.Unmarshal(block.Input, &ti)
			event["summary"] = toolSummary(block.Name, ti)
		case "text":
			event["text"] = block.Text
		case "thinking":
			event["thinking"] = block.Thinking
		case "tool_result":
			event["result"] = block.Text
		}
		out, _ := json.Marshal(event)
		fmt.Println(string(out))
	}
}

func ts() string {
	return time.Now().Format("15:04:05")
}

func truncate(s string, max int) string {
	// Collapse to single line.
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
