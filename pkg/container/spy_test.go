package container

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"over limit", "hello world", 5, "hello..."},
		{"empty", "", 10, ""},
		{"newlines collapsed", "line1\nline2\nline3", 50, "line1 line2 line3"},
		{"newlines then truncated", "line1\nline2\nline3", 10, "line1 line..."},
		{"leading/trailing whitespace", "  hello  ", 10, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.in, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

func TestToolSummary(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    toolInput
		want     string
	}{
		{"bash command", "Bash", toolInput{Command: "go test ./..."}, "go test ./..."},
		{"read file", "Read", toolInput{FilePath: "/src/main.go"}, "/src/main.go"},
		{"write file", "Write", toolInput{FilePath: "/src/new.go"}, "/src/new.go"},
		{"edit file", "Edit", toolInput{FilePath: "/src/fix.go"}, "/src/fix.go"},
		{"glob pattern", "Glob", toolInput{Pattern: "**/*.go"}, "**/*.go"},
		{"grep pattern", "Grep", toolInput{Pattern: "func main"}, "func main"},
		{"web fetch", "WebFetch", toolInput{URL: "https://example.com"}, "https://example.com"},
		{"web search", "WebSearch", toolInput{Query: "golang testing"}, "golang testing"},
		{"task", "Task", toolInput{Content: "explore the codebase"}, "explore the codebase"},
		{"unknown with filepath", "CustomTool", toolInput{FilePath: "/path"}, "/path"},
		{"unknown with command", "CustomTool", toolInput{Command: "ls"}, "ls"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolSummary(tt.toolName, tt.input)
			if got != tt.want {
				t.Errorf("toolSummary(%q, ...) = %q, want %q", tt.toolName, got, tt.want)
			}
		})
	}
}

func TestRenderLine_InvalidJSON(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	renderLine("not valid json", SpyOptions{})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "not valid json") {
		t.Errorf("expected invalid JSON to be printed as-is, got: %q", output)
	}
}

func TestRenderLine_ToolUse(t *testing.T) {
	inputJSON, _ := json.Marshal(toolInput{Command: "go build ./..."})
	msg := jsonlMessage{
		Message: &messageBody{
			Role: "assistant",
			Content: []contentBlock{
				{
					Type:  "tool_use",
					Name:  "Bash",
					Input: inputJSON,
				},
			},
		},
	}
	line, _ := json.Marshal(msg)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	renderLine(string(line), SpyOptions{})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Bash") {
		t.Errorf("expected tool name 'Bash' in output, got: %q", output)
	}
	if !strings.Contains(output, "go build ./...") {
		t.Errorf("expected command in output, got: %q", output)
	}
}

func TestRenderLine_TextBlock(t *testing.T) {
	msg := jsonlMessage{
		Message: &messageBody{
			Role: "assistant",
			Content: []contentBlock{
				{
					Type: "text",
					Text: "Hello from Claude",
				},
			},
		},
	}
	line, _ := json.Marshal(msg)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	renderLine(string(line), SpyOptions{})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Hello from Claude") {
		t.Errorf("expected text in output, got: %q", output)
	}
}

func TestRenderLine_ToolsOnlyFilter(t *testing.T) {
	msg := jsonlMessage{
		Message: &messageBody{
			Role: "assistant",
			Content: []contentBlock{
				{
					Type: "text",
					Text: "This should be filtered",
				},
			},
		},
	}
	line, _ := json.Marshal(msg)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	renderLine(string(line), SpyOptions{ToolsOnly: true})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if strings.Contains(output, "This should be filtered") {
		t.Errorf("text should be filtered in tools-only mode, got: %q", output)
	}
}

func TestRenderLine_ThinkingHiddenByDefault(t *testing.T) {
	msg := jsonlMessage{
		Message: &messageBody{
			Role: "assistant",
			Content: []contentBlock{
				{
					Type:     "thinking",
					Thinking: "Internal reasoning",
				},
			},
		},
	}
	line, _ := json.Marshal(msg)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	renderLine(string(line), SpyOptions{Thinking: false})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if strings.Contains(output, "Internal reasoning") {
		t.Errorf("thinking should be hidden by default, got: %q", output)
	}
}

func TestRenderLine_ThinkingShownWhenEnabled(t *testing.T) {
	msg := jsonlMessage{
		Message: &messageBody{
			Role: "assistant",
			Content: []contentBlock{
				{
					Type:     "thinking",
					Thinking: "Internal reasoning",
				},
			},
		},
	}
	line, _ := json.Marshal(msg)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	renderLine(string(line), SpyOptions{Thinking: true})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Internal reasoning") {
		t.Errorf("thinking should be shown when enabled, got: %q", output)
	}
}

func TestRenderLine_JSONMode(t *testing.T) {
	inputJSON, _ := json.Marshal(toolInput{FilePath: "/src/main.go"})
	msg := jsonlMessage{
		Message: &messageBody{
			Role: "assistant",
			Content: []contentBlock{
				{
					Type:  "tool_use",
					Name:  "Read",
					Input: inputJSON,
				},
			},
		},
	}
	line, _ := json.Marshal(msg)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	renderLine(string(line), SpyOptions{JSON: true})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Should be valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
		t.Fatalf("expected valid JSON output, got: %q, err: %v", output, err)
	}

	if result["type"] != "tool_use" {
		t.Errorf("expected type=tool_use, got: %v", result["type"])
	}
	if result["tool"] != "Read" {
		t.Errorf("expected tool=Read, got: %v", result["tool"])
	}
	if result["summary"] != "/src/main.go" {
		t.Errorf("expected summary=/src/main.go, got: %v", result["summary"])
	}
}

func TestRenderLine_ProgressToolsOnlyFilter(t *testing.T) {
	pd, _ := json.Marshal(progressData{Type: "bash_progress", ElapsedTimeSeconds: 5, TotalLines: 10})
	msg := jsonlMessage{
		Type: "progress",
		Data: pd,
	}
	line, _ := json.Marshal(msg)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	renderLine(string(line), SpyOptions{ToolsOnly: true})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if strings.Contains(output, "running") {
		t.Errorf("progress should be filtered in tools-only mode, got: %q", output)
	}
}

func TestRenderLine_ToolResultHiddenByDefault(t *testing.T) {
	msg := jsonlMessage{
		Message: &messageBody{
			Role: "user",
			Content: []contentBlock{
				{
					Type: "tool_result",
					Text: "command output here",
				},
			},
		},
	}
	line, _ := json.Marshal(msg)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	renderLine(string(line), SpyOptions{Verbose: false})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if strings.Contains(output, "command output here") {
		t.Errorf("tool_result should be hidden by default, got: %q", output)
	}
}

func TestRenderLine_ToolResultShownWhenVerbose(t *testing.T) {
	msg := jsonlMessage{
		Message: &messageBody{
			Role: "user",
			Content: []contentBlock{
				{
					Type: "tool_result",
					Text: "command output here",
				},
			},
		},
	}
	line, _ := json.Marshal(msg)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	renderLine(string(line), SpyOptions{Verbose: true})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "command output here") {
		t.Errorf("tool_result should be shown when verbose, got: %q", output)
	}
}

func TestSpyOptions_Defaults(t *testing.T) {
	opts := SpyOptions{}
	if opts.Raw || opts.ToolsOnly || opts.Thinking || opts.Verbose || opts.JSON {
		t.Error("default SpyOptions should have all fields false")
	}
}

func TestClaudeConfigParsing(t *testing.T) {
	raw := `{
		"projects": {
			"/home/agent/repo": {
				"lastSessionId": "abc-123-def"
			}
		}
	}`

	var cfg claudeConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("failed to parse claude config: %v", err)
	}

	proj, ok := cfg.Projects["/home/agent/repo"]
	if !ok {
		t.Fatal("expected project entry for /home/agent/repo")
	}
	if proj.LastSessionID != "abc-123-def" {
		t.Errorf("expected lastSessionId=abc-123-def, got: %s", proj.LastSessionID)
	}
}

func TestJsonlMessageParsing(t *testing.T) {
	inputJSON, _ := json.Marshal(toolInput{Command: "ls -la"})
	raw := `{
		"type": "message",
		"message": {
			"role": "assistant",
			"content": [
				{"type": "tool_use", "name": "Bash", "input": ` + string(inputJSON) + `},
				{"type": "text", "text": "Let me check"}
			]
		}
	}`

	var msg jsonlMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("failed to parse JSONL message: %v", err)
	}

	if msg.Message == nil {
		t.Fatal("expected message body")
	}
	if msg.Message.Role != "assistant" {
		t.Errorf("expected role=assistant, got: %s", msg.Message.Role)
	}
	if len(msg.Message.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got: %d", len(msg.Message.Content))
	}
	if msg.Message.Content[0].Type != "tool_use" {
		t.Errorf("expected first block type=tool_use, got: %s", msg.Message.Content[0].Type)
	}
	if msg.Message.Content[0].Name != "Bash" {
		t.Errorf("expected tool name=Bash, got: %s", msg.Message.Content[0].Name)
	}
	if msg.Message.Content[1].Type != "text" {
		t.Errorf("expected second block type=text, got: %s", msg.Message.Content[1].Type)
	}
}
