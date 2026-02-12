package container

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryDir(t *testing.T) {
	dir := historyDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}
	expected := filepath.Join(home, ".agentctl", "history")
	if dir != expected {
		t.Errorf("historyDir() = %q, want %q", dir, expected)
	}
}

func TestSaveAndLoadHistory(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	h := &AgentHistory{
		Name:        "test-agent",
		Repo:        "https://github.com/test/repo",
		Branch:      "main",
		Intent:      "fix tests",
		Created:     time.Now().Add(-1 * time.Hour),
		CompletedAt: time.Now(),
		RemovedAt:   time.Now(),
		Result:      "success",
		Attempts:    3,
		Metadata: map[string]string{
			"pr_url": "https://github.com/test/repo/pull/1",
		},
	}

	err := SaveHistory(h)
	if err != nil {
		t.Fatalf("SaveHistory() error: %v", err)
	}

	loaded, err := LoadHistory("test-agent")
	if err != nil {
		t.Fatalf("LoadHistory() error: %v", err)
	}

	if loaded.Name != h.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, h.Name)
	}
	if loaded.Repo != h.Repo {
		t.Errorf("Repo = %q, want %q", loaded.Repo, h.Repo)
	}
	if loaded.Branch != h.Branch {
		t.Errorf("Branch = %q, want %q", loaded.Branch, h.Branch)
	}
	if loaded.Intent != h.Intent {
		t.Errorf("Intent = %q, want %q", loaded.Intent, h.Intent)
	}
	if loaded.Result != h.Result {
		t.Errorf("Result = %q, want %q", loaded.Result, h.Result)
	}
	if loaded.Attempts != h.Attempts {
		t.Errorf("Attempts = %d, want %d", loaded.Attempts, h.Attempts)
	}
	if loaded.Metadata["pr_url"] != h.Metadata["pr_url"] {
		t.Errorf("Metadata[pr_url] = %q, want %q", loaded.Metadata["pr_url"], h.Metadata["pr_url"])
	}
}

func TestLoadHistoryNotFound(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	_, err := LoadHistory("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent history, got nil")
	}
}

func TestListHistory(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Empty history should return nil, nil
	records, err := ListHistory()
	if err != nil {
		t.Fatalf("ListHistory() error on empty: %v", err)
	}
	if records != nil {
		t.Errorf("expected nil for empty history, got %d records", len(records))
	}

	// Save two records
	h1 := &AgentHistory{Name: "agent-1", Result: "success"}
	h2 := &AgentHistory{Name: "agent-2", Result: "failed"}
	SaveHistory(h1)
	SaveHistory(h2)

	records, err = ListHistory()
	if err != nil {
		t.Fatalf("ListHistory() error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("ListHistory() returned %d records, want 2", len(records))
	}
}

func TestSaveHistoryOverwrite(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	h := &AgentHistory{Name: "agent-1", Result: "failed"}
	SaveHistory(h)

	h.Result = "success"
	SaveHistory(h)

	loaded, _ := LoadHistory("agent-1")
	if loaded.Result != "success" {
		t.Errorf("Result = %q, want %q after overwrite", loaded.Result, "success")
	}
}

func TestSaveHistoryWithNilMetadata(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	h := &AgentHistory{Name: "agent-nil-meta", Result: "success", Metadata: nil}
	err := SaveHistory(h)
	if err != nil {
		t.Fatalf("SaveHistory() with nil metadata error: %v", err)
	}

	loaded, err := LoadHistory("agent-nil-meta")
	if err != nil {
		t.Fatalf("LoadHistory() error: %v", err)
	}
	if loaded.Name != "agent-nil-meta" {
		t.Errorf("Name = %q, want %q", loaded.Name, "agent-nil-meta")
	}
}

func TestHistoryDirCreation(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// History dir shouldn't exist yet
	hDir := filepath.Join(tmpHome, ".agentctl", "history")
	if _, err := os.Stat(hDir); !os.IsNotExist(err) {
		t.Fatal("history dir should not exist before SaveHistory")
	}

	SaveHistory(&AgentHistory{Name: "test"})

	// Now it should exist
	info, err := os.Stat(hDir)
	if err != nil {
		t.Fatalf("history dir should exist after SaveHistory: %v", err)
	}
	if !info.IsDir() {
		t.Error("history path should be a directory")
	}
}

func TestAgentLifecycleStates(t *testing.T) {
	// Verify the constants are distinct
	states := []AgentLifecycleState{StateActive, StateCompleted, StateExited, StateStopped}
	seen := make(map[AgentLifecycleState]bool)
	for _, s := range states {
		if seen[s] {
			t.Errorf("duplicate lifecycle state: %s", s)
		}
		seen[s] = true
	}
}

func TestCleanupPreservesHistory(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Create an agent metadata file
	agent := &Agent{
		Name:        "cleanup-test",
		ContainerID: "abc123",
		Port:        8042,
		Repo:        "https://github.com/test/repo",
		Branch:      "main",
		Status:      "exited",
		Created:     time.Now().Add(-2 * time.Hour),
	}
	saveAgent(agent)

	// Verify agent file exists
	if _, err := loadAgent("cleanup-test"); err != nil {
		t.Fatalf("agent should exist before cleanup: %v", err)
	}

	// Cleanup (podman commands will fail silently in test, which is fine)
	err := Cleanup("cleanup-test", "success", 2, map[string]string{"pr": "https://example.com/pr/1"})
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	// Agent metadata should be gone
	if _, err := loadAgent("cleanup-test"); err == nil {
		t.Error("agent metadata should be removed after cleanup")
	}

	// History should be preserved
	h, err := LoadHistory("cleanup-test")
	if err != nil {
		t.Fatalf("history should exist after cleanup: %v", err)
	}
	if h.Result != "success" {
		t.Errorf("Result = %q, want %q", h.Result, "success")
	}
	if h.Attempts != 2 {
		t.Errorf("Attempts = %d, want %d", h.Attempts, 2)
	}
	if h.Metadata["pr"] != "https://example.com/pr/1" {
		t.Errorf("Metadata[pr] = %q, want %q", h.Metadata["pr"], "https://example.com/pr/1")
	}
	if h.Repo != "https://github.com/test/repo" {
		t.Errorf("Repo = %q, want %q", h.Repo, "https://github.com/test/repo")
	}
}

func TestCleanupAgentNotFound(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	err := Cleanup("nonexistent", "killed", 0, nil)
	if err == nil {
		t.Error("expected error for nonexistent agent cleanup")
	}
}

func TestDefaultGracePeriod(t *testing.T) {
	if DefaultGracePeriod != 1*time.Hour {
		t.Errorf("DefaultGracePeriod = %v, want 1h", DefaultGracePeriod)
	}
}
