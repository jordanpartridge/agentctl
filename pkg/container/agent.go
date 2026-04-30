package container

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Agent struct {
	Name     string    `json:"name"`
	CloneDir string    `json:"clone_dir"`
	Repo     string    `json:"repo"`
	Branch   string    `json:"branch"`
	Status   string    `json:"status"`
	Created  time.Time `json:"created"`
	Intent   string    `json:"intent,omitempty"`
}

func agentsDir() string {
	return "/tmp/agents"
}

func cloneDir(name string) string {
	return filepath.Join(agentsDir(), name)
}

func logFile(name string) string {
	return filepath.Join(agentsDir(), name+".log")
}

// SpawnWithIntent clones the repo and optionally stores an intent description.
func SpawnWithIntent(name, repo, branch, intent, _ string) (*Agent, error) {
	agent, err := Spawn(name, repo, branch)
	if err != nil {
		return nil, err
	}
	if intent != "" {
		agent.Intent = intent
		saveAgent(agent)
	}
	return agent, nil
}

// Spawn clones the repo into /tmp/agents/<name> and records metadata.
func Spawn(name, repo, branch string) (*Agent, error) {
	dir := cloneDir(name)

	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("agent %s already exists at %s", name, dir)
	}

	if err := os.MkdirAll(agentsDir(), 0755); err != nil {
		return nil, fmt.Errorf("failed to create agents dir: %w", err)
	}

	cloneURL := repo
	if ghToken := githubToken(); ghToken != "" && strings.HasPrefix(repo, "https://") {
		cloneURL = strings.Replace(repo, "https://", fmt.Sprintf("https://%s@", ghToken), 1)
	}

	fmt.Printf("📥 Cloning %s → %s\n", repo, dir)
	out, err := exec.Command("git", "clone", "--depth=1", cloneURL, dir).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("clone failed: %s", strings.TrimSpace(string(out)))
	}

	if branch != "" && branch != "master" && branch != "main" {
		exec.Command("git", "-C", dir, "checkout", branch).Run()
	}

	if branch == "" {
		branch = "master"
	}

	agent := &Agent{
		Name:     name,
		CloneDir: dir,
		Repo:     repo,
		Branch:   branch,
		Status:   "running",
		Created:  time.Now(),
	}
	saveAgent(agent)
	return agent, nil
}

// Kill removes the clone directory and metadata.
func Kill(name string) error {
	agent, _ := loadAgent(name)
	if agent != nil && agent.CloneDir != "" {
		os.RemoveAll(agent.CloneDir)
	}
	os.Remove(logFile(name))
	os.Remove(agentMetaPath(name))
	fmt.Printf("Killed: %s\n", name)
	return nil
}

// List returns all managed agents.
func List() ([]*Agent, error) {
	entries, _ := os.ReadDir(agentDir())
	var agents []*Agent
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(agentDir(), e.Name()))
		var agent Agent
		json.Unmarshal(data, &agent)

		// Mark as exited if clone directory is gone
		if _, err := os.Stat(agent.CloneDir); err != nil {
			agent.Status = "exited"
		}
		agents = append(agents, &agent)
	}
	return agents, nil
}

// Status prints agent details.
func Status(name string) error {
	agent, err := loadAgent(name)
	if err != nil {
		return err
	}
	fmt.Printf("Agent:    %s\n", agent.Name)
	fmt.Printf("Clone:    %s\n", agent.CloneDir)
	fmt.Printf("Repo:     %s\n", agent.Repo)
	fmt.Printf("Branch:   %s\n", agent.Branch)
	fmt.Printf("Created:  %s\n", agent.Created.Format(time.RFC3339))
	if agent.Intent != "" {
		fmt.Printf("Intent:   %s\n", agent.Intent)
	}
	return nil
}

// Logs prints the agent's run log.
func Logs(name string) error {
	f := logFile(name)
	data, err := os.ReadFile(f)
	if err != nil {
		return fmt.Errorf("no log found at %s", f)
	}
	fmt.Print(string(data))
	return nil
}

// LogsFollow streams the agent's run log in real-time.
func LogsFollow(name string) error {
	cmd := exec.Command("tail", "-f", logFile(name))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Shell opens an interactive shell in the agent's clone directory.
func Shell(name string) error {
	agent, err := loadAgent(name)
	if err != nil {
		return err
	}
	cmd := exec.Command("/bin/bash")
	cmd.Dir = agent.CloneDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Diagnose prints diagnostic info about an agent.
func Diagnose(name string) (*DiagnoseInfo, error) {
	agent, err := loadAgent(name)
	if err != nil {
		return nil, err
	}

	info := &DiagnoseInfo{AuthFiles: make(map[string]bool)}

	// Git status
	out, _ := exec.Command("git", "-C", agent.CloneDir, "status").Output()
	info.Processes = string(out)

	// Log tail
	if data, err := os.ReadFile(logFile(name)); err == nil {
		lines := strings.Split(string(data), "\n")
		start := 0
		if len(lines) > 20 {
			start = len(lines) - 20
		}
		info.ErrorLogs = strings.Join(lines[start:], "\n")
	} else {
		info.ErrorLogs = "No log file found"
	}

	// Check if run-task process is active
	out, _ = exec.Command("sh", "-c",
		fmt.Sprintf("ps aux | grep -v grep | grep '%s' || true", agent.CloneDir)).Output()
	info.ClaudeRunning = len(strings.TrimSpace(string(out))) > 0

	// Disk usage
	out, _ = exec.Command("du", "-sh", agent.CloneDir).Output()
	info.DiskSpace = strings.TrimSpace(string(out))

	// Available tools
	tools := []string{"claude", "git", "gh", "php", "composer", "node", "npm", "go", "python3"}
	for _, tool := range tools {
		if err := exec.Command("which", tool).Run(); err == nil {
			info.AvailableTools = append(info.AvailableTools, tool)
		}
	}

	return info, nil
}

// DiagnoseInfo contains diagnostic information about an agent.
type DiagnoseInfo struct {
	Processes      string
	ClaudeRunning  bool
	ErrorLogs      string
	AuthFiles      map[string]bool
	DiskSpace      string
	AvailableTools []string
}

// Prune removes agents whose clone directories no longer exist.
func Prune() ([]string, error) {
	agents, err := List()
	if err != nil {
		return nil, err
	}
	var pruned []string
	for _, a := range agents {
		if a.Status == "exited" {
			os.Remove(agentMetaPath(a.Name))
			pruned = append(pruned, a.Name)
		}
	}
	return pruned, nil
}

func githubToken() string {
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func agentDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agentctl", "agents")
}

func agentMetaPath(name string) string {
	return filepath.Join(agentDir(), name+".json")
}

func saveAgent(agent *Agent) error {
	os.MkdirAll(agentDir(), 0755)
	data, _ := json.MarshalIndent(agent, "", "  ")
	return os.WriteFile(agentMetaPath(agent.Name), data, 0644)
}

func loadAgent(name string) (*Agent, error) {
	data, err := os.ReadFile(agentMetaPath(name))
	if err != nil {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	var agent Agent
	json.Unmarshal(data, &agent)
	return &agent, nil
}
