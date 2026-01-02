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

// Spawn creates a new agent container with the given repo cloned
func Spawn(name, repo, branch string) (*Agent, error) {
	rand.Seed(time.Now().UnixNano())
	port := 8000 + rand.Intn(1000)

	// Get GitHub token from environment or gh CLI
	ghToken := os.Getenv("GH_TOKEN")
	if ghToken == "" {
		out, err := exec.Command("gh", "auth", "token").Output()
		if err == nil {
			ghToken = strings.TrimSpace(string(out))
		}
	}

	args := []string{
		"run", "-d",
		"--name", name,
		"-p", fmt.Sprintf("%d:8080", port),
		"-e", fmt.Sprintf("GH_TOKEN=%s", ghToken),
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

// Shell opens an interactive shell in the agent container
func Shell(name string) error {
	cmd := exec.Command("podman", "exec", "-it", name, "/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
