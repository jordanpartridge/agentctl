package coordination

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AgentState represents the coordination state of a single agent.
type AgentState struct {
	Name       string    `json:"name"`
	Branch     string    `json:"branch,omitempty"`
	Status     string    `json:"status"` // "working", "idle", "done", "blocked"
	LastUpdate time.Time `json:"last_update"`
}

// State represents the shared coordination state for a repo.
type State struct {
	Agents      map[string]*AgentState `json:"agents"`
	LastUpdated string                 `json:"last_updated"`
}

// UpdateAgentState updates an agent's state in the shared state file.
func UpdateAgentState(repoURL, agentName, status, branch string) error {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return err
	}

	state, err := loadState(dir)
	if err != nil {
		return err
	}

	state.Agents[agentName] = &AgentState{
		Name:       agentName,
		Branch:     branch,
		Status:     status,
		LastUpdate: time.Now(),
	}
	state.LastUpdated = time.Now().Format(time.RFC3339)

	return saveState(dir, state)
}

// RemoveAgentState removes an agent from the shared state.
func RemoveAgentState(repoURL, agentName string) error {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return err
	}

	state, err := loadState(dir)
	if err != nil {
		return err
	}

	delete(state.Agents, agentName)
	state.LastUpdated = time.Now().Format(time.RFC3339)

	return saveState(dir, state)
}

// GetState returns the current coordination state.
func GetState(repoURL string) (*State, error) {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return nil, err
	}
	return loadState(dir)
}

func loadState(dir string) (*State, error) {
	statePath := filepath.Join(dir, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Agents: make(map[string]*AgentState)}, nil
		}
		return nil, fmt.Errorf("cannot read state.json: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("cannot parse state.json: %w", err)
	}

	if state.Agents == nil {
		state.Agents = make(map[string]*AgentState)
	}
	return &state, nil
}

func saveState(dir string, state *State) error {
	statePath := filepath.Join(dir, "state.json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal state: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(statePath, data, 0644)
}
