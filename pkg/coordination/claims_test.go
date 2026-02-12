package coordination

import (
	"os"
	"testing"
)

func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return repoURL, func() { os.RemoveAll(dir) }
}

func TestClaimFile(t *testing.T) {
	repoURL, cleanup := setupTestRepo(t)
	defer cleanup()

	err := ClaimFile(repoURL, "agent-1", "src/main.go")
	if err != nil {
		t.Fatalf("ClaimFile failed: %v", err)
	}

	// Verify claim exists
	claims, err := ListClaims(repoURL)
	if err != nil {
		t.Fatalf("ListClaims failed: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}
	if claims["src/main.go"].Agent != "agent-1" {
		t.Error("claim should be held by agent-1")
	}
}

func TestClaimFileIdempotent(t *testing.T) {
	repoURL, cleanup := setupTestRepo(t)
	defer cleanup()

	// Claiming the same file twice by the same agent should succeed
	if err := ClaimFile(repoURL, "agent-1", "src/main.go"); err != nil {
		t.Fatalf("first claim failed: %v", err)
	}
	if err := ClaimFile(repoURL, "agent-1", "src/main.go"); err != nil {
		t.Fatalf("idempotent claim failed: %v", err)
	}
}

func TestClaimFileConflict(t *testing.T) {
	repoURL, cleanup := setupTestRepo(t)
	defer cleanup()

	if err := ClaimFile(repoURL, "agent-1", "src/main.go"); err != nil {
		t.Fatalf("first claim failed: %v", err)
	}

	// Different agent claiming same file should fail
	err := ClaimFile(repoURL, "agent-2", "src/main.go")
	if err == nil {
		t.Error("expected error when different agent claims same file")
	}
}

func TestReleaseFile(t *testing.T) {
	repoURL, cleanup := setupTestRepo(t)
	defer cleanup()

	if err := ClaimFile(repoURL, "agent-1", "src/main.go"); err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	if err := ReleaseFile(repoURL, "agent-1", "src/main.go"); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	claims, err := ListClaims(repoURL)
	if err != nil {
		t.Fatalf("ListClaims failed: %v", err)
	}
	if len(claims) != 0 {
		t.Errorf("expected 0 claims after release, got %d", len(claims))
	}
}

func TestReleaseFileWrongAgent(t *testing.T) {
	repoURL, cleanup := setupTestRepo(t)
	defer cleanup()

	if err := ClaimFile(repoURL, "agent-1", "src/main.go"); err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	err := ReleaseFile(repoURL, "agent-2", "src/main.go")
	if err == nil {
		t.Error("expected error when wrong agent releases file")
	}
}

func TestReleaseUnclaimedFile(t *testing.T) {
	repoURL, cleanup := setupTestRepo(t)
	defer cleanup()

	// Releasing an unclaimed file should succeed (no-op)
	if err := ReleaseFile(repoURL, "agent-1", "src/main.go"); err != nil {
		t.Fatalf("releasing unclaimed file should not error: %v", err)
	}
}

func TestIsFileClaimed(t *testing.T) {
	repoURL, cleanup := setupTestRepo(t)
	defer cleanup()

	// Initially unclaimed
	agent, claimed, err := IsFileClaimed(repoURL, "src/main.go")
	if err != nil {
		t.Fatalf("IsFileClaimed failed: %v", err)
	}
	if claimed {
		t.Error("file should not be claimed initially")
	}
	if agent != "" {
		t.Error("agent should be empty when unclaimed")
	}

	// Claim it
	if err := ClaimFile(repoURL, "agent-1", "src/main.go"); err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	agent, claimed, err = IsFileClaimed(repoURL, "src/main.go")
	if err != nil {
		t.Fatalf("IsFileClaimed failed: %v", err)
	}
	if !claimed {
		t.Error("file should be claimed")
	}
	if agent != "agent-1" {
		t.Errorf("expected agent-1, got %s", agent)
	}
}

func TestReleaseAllForAgent(t *testing.T) {
	repoURL, cleanup := setupTestRepo(t)
	defer cleanup()

	// Claim multiple files
	ClaimFile(repoURL, "agent-1", "file1.go")
	ClaimFile(repoURL, "agent-1", "file2.go")
	ClaimFile(repoURL, "agent-2", "file3.go")

	if err := ReleaseAllForAgent(repoURL, "agent-1"); err != nil {
		t.Fatalf("ReleaseAllForAgent failed: %v", err)
	}

	claims, _ := ListClaims(repoURL)
	if len(claims) != 1 {
		t.Errorf("expected 1 claim remaining, got %d", len(claims))
	}
	if _, ok := claims["file3.go"]; !ok {
		t.Error("agent-2's claim should still exist")
	}
}
