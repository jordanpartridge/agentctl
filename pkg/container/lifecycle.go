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

// DefaultGracePeriod is how long a completed agent container stays before auto-cleanup.
const DefaultGracePeriod = 1 * time.Hour

// AgentHistory preserves metadata about an agent after its container is removed.
type AgentHistory struct {
	Name        string            `json:"name"`
	Repo        string            `json:"repo"`
	Branch      string            `json:"branch"`
	Intent      string            `json:"intent,omitempty"`
	Created     time.Time         `json:"created"`
	CompletedAt time.Time         `json:"completed_at,omitempty"`
	RemovedAt   time.Time         `json:"removed_at,omitempty"`
	Result      string            `json:"result"` // "success", "failed", "killed"
	Attempts    int               `json:"attempts,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"` // PR URL, commit SHA, etc.
}

// historyDir returns the path to the agent history directory.
func historyDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agentctl", "history")
}

func historyPath(name string) string {
	return filepath.Join(historyDir(), name+".json")
}

// SaveHistory persists an agent history record.
func SaveHistory(h *AgentHistory) error {
	if err := os.MkdirAll(historyDir(), 0755); err != nil {
		return fmt.Errorf("failed to create history dir: %w", err)
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}
	return os.WriteFile(historyPath(h.Name), data, 0644)
}

// LoadHistory loads a single agent history record.
func LoadHistory(name string) (*AgentHistory, error) {
	data, err := os.ReadFile(historyPath(name))
	if err != nil {
		return nil, fmt.Errorf("history not found: %s", name)
	}
	var h AgentHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("failed to parse history: %w", err)
	}
	return &h, nil
}

// ListHistory returns all agent history records.
func ListHistory() ([]*AgentHistory, error) {
	entries, err := os.ReadDir(historyDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []*AgentHistory
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(historyDir(), e.Name()))
		if err != nil {
			continue
		}
		var h AgentHistory
		if err := json.Unmarshal(data, &h); err != nil {
			continue
		}
		records = append(records, &h)
	}
	return records, nil
}

// AgentLifecycleState categorizes an agent's current lifecycle phase.
type AgentLifecycleState string

const (
	StateActive    AgentLifecycleState = "active"    // Claude is running, work in progress
	StateCompleted AgentLifecycleState = "completed" // Task done, awaiting cleanup
	StateExited    AgentLifecycleState = "exited"    // Container exited (may be stale)
	StateStopped   AgentLifecycleState = "stopped"   // Container not found
)

// AgentWithState enriches an Agent with lifecycle information.
type AgentWithState struct {
	*Agent
	Lifecycle   AgentLifecycleState `json:"lifecycle"`
	ContainerUp bool                `json:"container_up"`
	Age         time.Duration       `json:"-"`
}

// ListWithState returns all agents enriched with lifecycle state.
func ListWithState() ([]*AgentWithState, error) {
	entries, _ := os.ReadDir(agentDir())
	var agents []*AgentWithState
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(agentDir(), e.Name()))
		var agent Agent
		if err := json.Unmarshal(data, &agent); err != nil {
			continue
		}

		aws := &AgentWithState{
			Agent: &agent,
			Age:   time.Since(agent.Created),
		}

		// Get container status from podman
		out, _ := exec.Command("podman", "inspect", "-f", "{{.State.Status}}", agent.Name).Output()
		containerStatus := strings.TrimSpace(string(out))

		switch containerStatus {
		case "running":
			aws.ContainerUp = true
			// Check if Claude is still working
			psOut, _ := exec.Command("podman", "exec", agent.Name, "sh", "-c",
				"ps aux 2>/dev/null | grep -v grep | grep claude || true").Output()
			if len(strings.TrimSpace(string(psOut))) > 0 {
				aws.Lifecycle = StateActive
			} else {
				aws.Lifecycle = StateCompleted
			}
		case "exited":
			aws.ContainerUp = false
			aws.Lifecycle = StateExited
		default:
			aws.ContainerUp = false
			aws.Lifecycle = StateStopped
		}

		agent.Status = containerStatus
		if agent.Status == "" {
			agent.Status = "stopped"
		}

		agents = append(agents, aws)
	}
	return agents, nil
}

// Cleanup stops and removes a single agent container, preserving history.
func Cleanup(name string, result string, attempts int, metadata map[string]string) error {
	agent, err := loadAgent(name)
	if err != nil {
		return fmt.Errorf("agent not found: %s", name)
	}

	// Save history before removing
	h := &AgentHistory{
		Name:        agent.Name,
		Repo:        agent.Repo,
		Branch:      agent.Branch,
		Intent:      agent.Intent,
		Created:     agent.Created,
		CompletedAt: time.Now(),
		RemovedAt:   time.Now(),
		Result:      result,
		Attempts:    attempts,
		Metadata:    metadata,
	}
	if err := SaveHistory(h); err != nil {
		return fmt.Errorf("failed to save history: %w", err)
	}

	// Stop and remove container
	exec.Command("podman", "stop", name).Run()
	exec.Command("podman", "rm", name).Run()

	// Remove agent metadata file
	os.Remove(agentMetaPath(name))

	return nil
}

// Prune removes all exited and stopped agent containers, preserving history.
func Prune() ([]string, error) {
	agents, err := ListWithState()
	if err != nil {
		return nil, err
	}

	var pruned []string
	for _, a := range agents {
		if a.Lifecycle == StateExited || a.Lifecycle == StateStopped {
			if err := Cleanup(a.Name, "pruned", 0, nil); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to prune %s: %v\n", a.Name, err)
				continue
			}
			pruned = append(pruned, a.Name)
		}
	}
	return pruned, nil
}

// CleanupCompleted removes completed agents that have exceeded the grace period.
func CleanupCompleted(gracePeriod time.Duration) ([]string, error) {
	agents, err := ListWithState()
	if err != nil {
		return nil, err
	}

	var cleaned []string
	for _, a := range agents {
		if a.Lifecycle == StateCompleted && a.Age > gracePeriod {
			if err := Cleanup(a.Name, "success", 0, nil); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to cleanup %s: %v\n", a.Name, err)
				continue
			}
			cleaned = append(cleaned, a.Name)
		}
	}
	return cleaned, nil
}

// CleanupStale removes containers that have been exited for longer than the grace period.
func CleanupStale(gracePeriod time.Duration) ([]string, error) {
	agents, err := ListWithState()
	if err != nil {
		return nil, err
	}

	var cleaned []string
	for _, a := range agents {
		if (a.Lifecycle == StateExited || a.Lifecycle == StateStopped) && a.Age > gracePeriod {
			if err := Cleanup(a.Name, "stale", 0, nil); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to cleanup %s: %v\n", a.Name, err)
				continue
			}
			cleaned = append(cleaned, a.Name)
		}
	}
	return cleaned, nil
}
