package container

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheDir(t *testing.T) {
	dir := cacheDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}
	expected := filepath.Join(home, ".agentctl", "cache")
	if dir != expected {
		t.Errorf("cacheDir() = %q, want %q", dir, expected)
	}
}

func TestEnsureCacheDirs(t *testing.T) {
	// Use a temp directory as home to avoid polluting real home
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	err := ensureCacheDirs()
	if err != nil {
		t.Fatalf("ensureCacheDirs() error: %v", err)
	}

	expectedDirs := []string{"composer", "npm", "go-mod", "pip"}
	cache := filepath.Join(tmpHome, ".agentctl", "cache")
	for _, d := range expectedDirs {
		path := filepath.Join(cache, d)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", d)
		}
	}
}

func TestEnsureCacheDirsIdempotent(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Call twice to verify idempotency
	if err := ensureCacheDirs(); err != nil {
		t.Fatalf("first ensureCacheDirs() error: %v", err)
	}
	if err := ensureCacheDirs(); err != nil {
		t.Fatalf("second ensureCacheDirs() error: %v", err)
	}
}

func TestAgentDir(t *testing.T) {
	dir := agentDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}
	expected := filepath.Join(home, ".agentctl", "agents")
	if dir != expected {
		t.Errorf("agentDir() = %q, want %q", dir, expected)
	}
}

func TestSaveAndLoadAgent(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	agent := &Agent{
		Name:        "test-agent",
		ContainerID: "abc123",
		Port:        8042,
		Repo:        "https://github.com/test/repo",
		Branch:      "main",
		Status:      "running",
	}

	err := saveAgent(agent)
	if err != nil {
		t.Fatalf("saveAgent() error: %v", err)
	}

	loaded, err := loadAgent("test-agent")
	if err != nil {
		t.Fatalf("loadAgent() error: %v", err)
	}

	if loaded.Name != agent.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, agent.Name)
	}
	if loaded.ContainerID != agent.ContainerID {
		t.Errorf("ContainerID = %q, want %q", loaded.ContainerID, agent.ContainerID)
	}
	if loaded.Port != agent.Port {
		t.Errorf("Port = %d, want %d", loaded.Port, agent.Port)
	}
	if loaded.Repo != agent.Repo {
		t.Errorf("Repo = %q, want %q", loaded.Repo, agent.Repo)
	}
	if loaded.Branch != agent.Branch {
		t.Errorf("Branch = %q, want %q", loaded.Branch, agent.Branch)
	}
}

func TestLoadAgentNotFound(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	_, err := loadAgent("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent, got nil")
	}
}
