package container

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Agent struct {
	Name        string    `json:"name"`
	ContainerID string    `json:"container_id"`
	Port        int       `json:"port"`
	Repo        string    `json:"repo"`
	Branch      string    `json:"branch"`
	Status      string    `json:"status"`
	Created     time.Time `json:"created"`
	Intent      string    `json:"intent,omitempty"`
}

// cacheDir returns the path to the shared cache directory on the host
func cacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agentctl", "cache")
}

// ensureCacheDirs creates the shared cache directories on the host if they don't exist
func ensureCacheDirs() error {
	dirs := []string{
		"composer",
		"npm",
		"go-mod",
		"pip",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(cacheDir(), d), 0755); err != nil {
			return fmt.Errorf("failed to create cache dir %s: %w", d, err)
		}
	}
	return nil
}

// Spawn creates a new agent container with the given repo cloned
func Spawn(name, repo, branch string) (*Agent, error) {
	rand.Seed(time.Now().UnixNano())
	port := 8000 + rand.Intn(1000)

	// Ensure shared cache directories exist on host
	if err := ensureCacheDirs(); err != nil {
		return nil, fmt.Errorf("cache setup failed: %w", err)
	}

	// Get GitHub token from environment or gh CLI
	ghToken := os.Getenv("GH_TOKEN")
	if ghToken == "" {
		out, err := exec.Command("gh", "auth", "token").Output()
		if err == nil {
			ghToken = strings.TrimSpace(string(out))
		}
	}

	cache := cacheDir()
	args := []string{
		"run", "-d",
		"--name", name,
		"-p", fmt.Sprintf("%d:8080", port),
		"-e", fmt.Sprintf("GH_TOKEN=%s", ghToken),
		"-v", fmt.Sprintf("%s/composer:/home/agent/.cache/composer:z", cache),
		"-v", fmt.Sprintf("%s/npm:/home/agent/.cache/npm:z", cache),
		"-v", fmt.Sprintf("%s/go-mod:/home/agent/.cache/go-mod:z", cache),
		"-v", fmt.Sprintf("%s/pip:/home/agent/.cache/pip:z", cache),
		"agent-devbox:latest",
	}

	cmd := exec.Command("podman", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("spawn failed: %w", err)
	}

	containerID := strings.TrimSpace(string(out))
	time.Sleep(2 * time.Second)

	// Copy Claude auth config from host
	home, _ := os.UserHomeDir()
	claudeJSON := filepath.Join(home, ".claude.json")
	claudeDir := filepath.Join(home, ".claude")

	if _, err := os.Stat(claudeJSON); err == nil {
		exec.Command("podman", "cp", claudeJSON, name+":/home/agent/.claude.json").Run()
		exec.Command("podman", "exec", name, "chown", "agent:agent", "/home/agent/.claude.json").Run()
	}
	if _, err := os.Stat(claudeDir); err == nil {
		exec.Command("podman", "cp", claudeDir, name+":/home/agent/.claude").Run()
		exec.Command("podman", "exec", name, "chown", "-R", "agent:agent", "/home/agent/.claude").Run()
	}

	// Clone the repository if provided
	if repo != "" {
		cloneURL := repo
		if ghToken != "" && strings.HasPrefix(repo, "https://") {
			cloneURL = strings.Replace(repo, "https://", fmt.Sprintf("https://%s@", ghToken), 1)
		}
		exec.Command("podman", "exec", name, "git", "clone", cloneURL, "/home/agent/workspace/repo").Run()
		exec.Command("podman", "exec", name, "sh", "-c",
			fmt.Sprintf("cd /home/agent/workspace/repo && git checkout %s 2>/dev/null || true", branch)).Run()
	}

	agent := &Agent{
		Name:        name,
		ContainerID: containerID,
		Port:        port,
		Repo:        repo,
		Branch:      branch,
		Status:      "running",
		Created:     time.Now(),
	}
	saveAgent(agent)
	return agent, nil
}

// Kill stops and removes an agent container
func Kill(name string) error {
	exec.Command("podman", "stop", name).Run()
	exec.Command("podman", "rm", name).Run()
	os.Remove(agentMetaPath(name))
	fmt.Printf("Killed: %s\n", name)
	return nil
}

// List returns all managed agents
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
		out, _ := exec.Command("podman", "inspect", "-f", "{{.State.Status}}", agent.Name).Output()
		agent.Status = strings.TrimSpace(string(out))
		if agent.Status == "" {
			agent.Status = "stopped"
		}
		agents = append(agents, &agent)
	}
	return agents, nil
}

// Status prints agent details
func Status(name string) error {
	agent, err := loadAgent(name)
	if err != nil {
		return err
	}
	out, _ := exec.Command("podman", "inspect", "-f", "{{.State.Status}}", name).Output()
	fmt.Printf("Agent: %s\n", agent.Name)
	fmt.Printf("Status: %s\n", strings.TrimSpace(string(out)))
	fmt.Printf("Port: %d\n", agent.Port)
	fmt.Printf("Repo: %s\n", agent.Repo)
	fmt.Printf("Branch: %s\n", agent.Branch)
	fmt.Printf("Created: %s\n", agent.Created.Format(time.RFC3339))
	return nil
}

// Logs shows Claude logs from the agent
func Logs(name string) error {
	cmd := exec.Command("podman", "exec", name, "cat", "/home/agent/claude.log")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LogsFollow streams Claude logs from the agent in real-time using tail -f
func LogsFollow(name string) error {
	cmd := exec.Command("podman", "exec", name, "tail", "-f", "/home/agent/claude.log")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Watch streams Claude's activity in real-time (alias for LogsFollow)
func Watch(name string) error {
	fmt.Printf("Watching agent %s (Ctrl+C to stop)...\n", name)
	fmt.Println("---")
	return LogsFollow(name)
}

// Shell opens an interactive shell in the agent container
func Shell(name string) error {
	cmd := exec.Command("podman", "exec", "-it", name, "/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// DiagnoseInfo contains diagnostic information about an agent
type DiagnoseInfo struct {
	Processes      string
	ClaudeRunning  bool
	ErrorLogs      string
	AuthFiles      map[string]bool
	DiskSpace      string
	AvailableTools []string
}

// Diagnose collects diagnostic information to help debug stuck agents
func Diagnose(name string) (*DiagnoseInfo, error) {
	info := &DiagnoseInfo{
		AuthFiles: make(map[string]bool),
	}

	// Get running processes
	out, _ := exec.Command("podman", "exec", name, "ps", "aux").Output()
	info.Processes = strings.TrimSpace(string(out))

	// Check if Claude is running
	out, _ = exec.Command("podman", "exec", name, "sh", "-c",
		"ps aux 2>/dev/null | grep -v grep | grep claude || true").Output()
	info.ClaudeRunning = len(strings.TrimSpace(string(out))) > 0

	// Get last 20 lines of error logs
	out, _ = exec.Command("podman", "exec", name, "sh", "-c",
		"tail -20 /home/agent/claude.log 2>/dev/null || echo 'No log file found'").Output()
	info.ErrorLogs = strings.TrimSpace(string(out))

	// Check if auth files exist
	authChecks := map[string]string{
		".claude.json": "/home/agent/.claude.json",
		".claude/":     "/home/agent/.claude",
	}
	for label, path := range authChecks {
		err := exec.Command("podman", "exec", name, "test", "-e", path).Run()
		info.AuthFiles[label] = err == nil
	}

	// Get disk space
	out, _ = exec.Command("podman", "exec", name, "df", "-h", "/home/agent").Output()
	info.DiskSpace = strings.TrimSpace(string(out))

	// Check available tools
	tools := []string{"claude", "git", "gh", "node", "npm", "go", "python3", "cargo"}
	for _, tool := range tools {
		err := exec.Command("podman", "exec", name, "which", tool).Run()
		if err == nil {
			info.AvailableTools = append(info.AvailableTools, tool)
		}
	}

	return info, nil
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
