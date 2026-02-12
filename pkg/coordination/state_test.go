package coordination

import (
	"os"
	"testing"
)

func TestUpdateAgentState(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	if err := UpdateAgentState(repoURL, "agent-1", "working", "feature-branch"); err != nil {
		t.Fatalf("UpdateAgentState failed: %v", err)
	}

	state, err := GetState(repoURL)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if len(state.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(state.Agents))
	}

	agent := state.Agents["agent-1"]
	if agent == nil {
		t.Fatal("agent-1 not found in state")
	}
	if agent.Name != "agent-1" {
		t.Errorf("expected name agent-1, got %s", agent.Name)
	}
	if agent.Status != "working" {
		t.Errorf("expected status working, got %s", agent.Status)
	}
	if agent.Branch != "feature-branch" {
		t.Errorf("expected branch feature-branch, got %s", agent.Branch)
	}
	if agent.LastUpdate.IsZero() {
		t.Error("LastUpdate should be set")
	}
}

func TestUpdateAgentStateOverwrite(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	UpdateAgentState(repoURL, "agent-1", "working", "branch-1")
	UpdateAgentState(repoURL, "agent-1", "done", "branch-1")

	state, _ := GetState(repoURL)
	if state.Agents["agent-1"].Status != "done" {
		t.Error("status should be updated to done")
	}
}

func TestMultipleAgentStates(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	UpdateAgentState(repoURL, "agent-1", "working", "branch-1")
	UpdateAgentState(repoURL, "agent-2", "idle", "branch-2")

	state, _ := GetState(repoURL)
	if len(state.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(state.Agents))
	}
}

func TestRemoveAgentState(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	UpdateAgentState(repoURL, "agent-1", "working", "branch-1")
	UpdateAgentState(repoURL, "agent-2", "idle", "branch-2")

	if err := RemoveAgentState(repoURL, "agent-1"); err != nil {
		t.Fatalf("RemoveAgentState failed: %v", err)
	}

	state, _ := GetState(repoURL)
	if len(state.Agents) != 1 {
		t.Fatalf("expected 1 agent after removal, got %d", len(state.Agents))
	}
	if _, ok := state.Agents["agent-1"]; ok {
		t.Error("agent-1 should have been removed")
	}
}

func TestGetStateEmpty(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	state, err := GetState(repoURL)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if len(state.Agents) != 0 {
		t.Errorf("expected 0 agents initially, got %d", len(state.Agents))
	}
}

func TestStateLastUpdated(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	UpdateAgentState(repoURL, "agent-1", "working", "main")

	state, _ := GetState(repoURL)
	if state.LastUpdated == "" {
		t.Error("LastUpdated should be set after update")
	}
}
