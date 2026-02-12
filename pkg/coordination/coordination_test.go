package coordination

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoHash(t *testing.T) {
	hash1 := repoHash("https://github.com/user/repo1")
	hash2 := repoHash("https://github.com/user/repo2")

	if hash1 == hash2 {
		t.Error("different repos should produce different hashes")
	}

	if len(hash1) != 12 {
		t.Errorf("hash should be 12 chars, got %d", len(hash1))
	}

	// Same input should produce same hash
	hash1again := repoHash("https://github.com/user/repo1")
	if hash1 != hash1again {
		t.Error("same repo should produce same hash")
	}
}

func TestCoordDir(t *testing.T) {
	dir, err := CoordDir("https://github.com/user/repo")
	if err != nil {
		t.Fatalf("CoordDir failed: %v", err)
	}

	if !filepath.IsAbs(dir) {
		t.Error("CoordDir should return absolute path")
	}

	if !contains(dir, ".agentctl") || !contains(dir, "coordination") {
		t.Errorf("unexpected dir path: %s", dir)
	}
}

func TestInit(t *testing.T) {
	repoURL := "https://github.com/test/init-test-" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	// Check that all files were created
	for _, file := range []string{"claims.json", "messages.jsonl", "state.json"} {
		path := filepath.Join(dir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Init should create %s", file)
		}
	}

	// Init should be idempotent
	dir2, err := Init(repoURL)
	if err != nil {
		t.Fatalf("second Init failed: %v", err)
	}
	if dir != dir2 {
		t.Error("Init should return the same directory on second call")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
