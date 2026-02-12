// Package coordination provides a shared file-based message bus for agent coordination.
// It manages file claims, inter-agent messaging, and shared state via a coordination
// directory at ~/.agentctl/coordination/<repo-hash>/.
package coordination

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// CoordDir returns the coordination directory for a given repo path.
// The directory is at ~/.agentctl/coordination/<repo-hash>/.
func CoordDir(repoURL string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	hash := repoHash(repoURL)
	dir := filepath.Join(home, ".agentctl", "coordination", hash)
	return dir, nil
}

// Init creates the coordination directory structure and initializes
// claims.json, messages.jsonl, and state.json if they don't exist.
func Init(repoURL string) (string, error) {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot create coordination directory: %w", err)
	}

	// Initialize claims.json if it doesn't exist
	claimsPath := filepath.Join(dir, "claims.json")
	if _, err := os.Stat(claimsPath); os.IsNotExist(err) {
		if err := os.WriteFile(claimsPath, []byte("{}\n"), 0644); err != nil {
			return "", fmt.Errorf("cannot create claims.json: %w", err)
		}
	}

	// Initialize messages.jsonl if it doesn't exist
	messagesPath := filepath.Join(dir, "messages.jsonl")
	if _, err := os.Stat(messagesPath); os.IsNotExist(err) {
		if err := os.WriteFile(messagesPath, []byte(""), 0644); err != nil {
			return "", fmt.Errorf("cannot create messages.jsonl: %w", err)
		}
	}

	// Initialize state.json if it doesn't exist
	statePath := filepath.Join(dir, "state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		initial := `{"agents":{},"last_updated":""}` + "\n"
		if err := os.WriteFile(statePath, []byte(initial), 0644); err != nil {
			return "", fmt.Errorf("cannot create state.json: %w", err)
		}
	}

	return dir, nil
}

// repoHash returns a short SHA-256 hash (first 12 chars) of the repo URL.
func repoHash(repoURL string) string {
	h := sha256.Sum256([]byte(repoURL))
	return hex.EncodeToString(h[:])[:12]
}
