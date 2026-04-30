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

// DefaultGracePeriod is how long a completed agent clone stays before auto-cleanup.
const DefaultGracePeriod = 1 * time.Hour

// AgentHistory preserves metadata about an agent after its clone is removed.
type AgentHistory struct {
	Name        string            `json:"name"`
	Repo        string            `json:"repo"`
	Branch      string            `json:"branch"`
	Intent      string            `json:"intent,omitempty"`
	Created     time.Time         `json:"created"`
	CompletedAt time.Time         `json:"completed_at,omitempty"`
	RemovedAt   time.Time         `json:"removed_at,omitempty"`
	Result      string            `json:"result"` // "success", "failed", "killed", "stale"
	Attempts    int               `json:"attempts,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

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
	data, _ := json.MarshalIndent(h, "", "  ")
	return os.WriteFile(historyPath(h.Name), data, 0644)
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
		data, _ := os.ReadFile(filepath.Join(historyDir(), e.Name()))
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
	StateActive    AgentLifecycleState = "active"    // run-task is running
	StateCompleted AgentLifecycleState = "completed" // clone exists, no active process
	StateExited    AgentLifecycleState = "exited"    // clone directory gone
)

// AgentWithState enriches an Agent with lifecycle information.
type AgentWithState struct {
	*Agent
	Lifecycle AgentLifecycleState
	Age       time.Duration
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

		if _, err := os.Stat(agent.CloneDir); err != nil {
			aws.Lifecycle = StateExited
		} else {
			// Active if a process is running inside the clone dir
			out, _ := exec.Command("sh", "-c",
				fmt.Sprintf("ps aux 2>/dev/null | grep -v grep | grep '%s' || true", agent.CloneDir)).Output()
			if len(strings.TrimSpace(string(out))) > 0 {
				aws.Lifecycle = StateActive
			} else {
				aws.Lifecycle = StateCompleted
			}
		}

		agents = append(agents, aws)
	}
	return agents, nil
}

// Cleanup removes a clone and saves history.
func Cleanup(name string, result string, attempts int, metadata map[string]string) error {
	agent, err := loadAgent(name)
	if err != nil {
		return fmt.Errorf("agent not found: %s", name)
	}

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
	SaveHistory(h)

	if h.Intent != "" {
		captureIntentKnowledge(h)
	}

	os.RemoveAll(agent.CloneDir)
	os.Remove(logFile(name))
	os.Remove(agentMetaPath(name))
	return nil
}

func captureIntentKnowledge(h *AgentHistory) {
	title := fmt.Sprintf("Agent %s: %s", h.Name, h.Result)
	content := fmt.Sprintf("Intent: %s\nRepo: %s\nBranch: %s\nResult: %s\nDuration: %s",
		h.Intent, h.Repo, h.Branch, h.Result,
		h.CompletedAt.Sub(h.Created).Round(time.Minute))
	exec.Command("know", "add", title, "--content", content,
		"--category", "deployment", "--tags", "agent,agentctl,"+h.Result,
		"--confidence", "80", "--no-git", "--skip-enhance", "--force", "-n").Run()
}

// CleanupCompleted removes clones that have been idle past the grace period.
func CleanupCompleted(gracePeriod time.Duration) ([]string, error) {
	agents, err := ListWithState()
	if err != nil {
		return nil, err
	}
	var cleaned []string
	for _, a := range agents {
		if a.Lifecycle == StateCompleted && a.Age > gracePeriod {
			if err := Cleanup(a.Name, "success", 0, nil); err == nil {
				cleaned = append(cleaned, a.Name)
			}
		}
	}
	return cleaned, nil
}

// CleanupStale removes exited agents past the grace period.
func CleanupStale(gracePeriod time.Duration) ([]string, error) {
	agents, err := ListWithState()
	if err != nil {
		return nil, err
	}
	var cleaned []string
	for _, a := range agents {
		if a.Lifecycle == StateExited && a.Age > gracePeriod {
			if err := Cleanup(a.Name, "stale", 0, nil); err == nil {
				cleaned = append(cleaned, a.Name)
			}
		}
	}
	return cleaned, nil
}
