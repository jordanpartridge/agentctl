package coordination

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Claim represents a file claim by an agent.
type Claim struct {
	Agent     string    `json:"agent"`
	File      string    `json:"file"`
	ClaimedAt time.Time `json:"claimed_at"`
}

// Claims is a map from file path to the Claim holding it.
type Claims map[string]*Claim

// ClaimFile attempts to claim a file for the given agent.
// Returns an error if the file is already claimed by another agent.
func ClaimFile(repoURL, agentName, filePath string) error {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return err
	}

	claims, err := loadClaims(dir)
	if err != nil {
		return err
	}

	if existing, ok := claims[filePath]; ok {
		if existing.Agent != agentName {
			return fmt.Errorf("file %s already claimed by agent %s (since %s)",
				filePath, existing.Agent, existing.ClaimedAt.Format(time.RFC3339))
		}
		// Already claimed by same agent, idempotent
		return nil
	}

	claims[filePath] = &Claim{
		Agent:     agentName,
		File:      filePath,
		ClaimedAt: time.Now(),
	}

	if err := saveClaims(dir, claims); err != nil {
		return err
	}

	// Publish a claim message on the bus
	return Publish(repoURL, Message{
		Type:  MsgClaim,
		Agent: agentName,
		Data:  map[string]string{"file": filePath},
	})
}

// ReleaseFile releases a file claim for the given agent.
// Returns an error if the file is claimed by a different agent.
func ReleaseFile(repoURL, agentName, filePath string) error {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return err
	}

	claims, err := loadClaims(dir)
	if err != nil {
		return err
	}

	existing, ok := claims[filePath]
	if !ok {
		// Not claimed, nothing to do
		return nil
	}

	if existing.Agent != agentName {
		return fmt.Errorf("file %s is claimed by agent %s, not %s",
			filePath, existing.Agent, agentName)
	}

	delete(claims, filePath)

	if err := saveClaims(dir, claims); err != nil {
		return err
	}

	// Publish a release message on the bus
	return Publish(repoURL, Message{
		Type:  MsgRelease,
		Agent: agentName,
		Data:  map[string]string{"file": filePath},
	})
}

// ListClaims returns all current file claims.
func ListClaims(repoURL string) (Claims, error) {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return nil, err
	}
	return loadClaims(dir)
}

// IsFileClaimed checks if a file is claimed by any agent.
// Returns the claiming agent name (empty if unclaimed) and whether it's claimed.
func IsFileClaimed(repoURL, filePath string) (string, bool, error) {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return "", false, err
	}

	claims, err := loadClaims(dir)
	if err != nil {
		return "", false, err
	}

	if claim, ok := claims[filePath]; ok {
		return claim.Agent, true, nil
	}
	return "", false, nil
}

// ReleaseAllForAgent releases all claims held by a given agent.
func ReleaseAllForAgent(repoURL, agentName string) error {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return err
	}

	claims, err := loadClaims(dir)
	if err != nil {
		return err
	}

	for file, claim := range claims {
		if claim.Agent == agentName {
			delete(claims, file)
		}
	}

	return saveClaims(dir, claims)
}

func loadClaims(dir string) (Claims, error) {
	claimsPath := filepath.Join(dir, "claims.json")
	data, err := os.ReadFile(claimsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(Claims), nil
		}
		return nil, fmt.Errorf("cannot read claims.json: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, fmt.Errorf("cannot parse claims.json: %w", err)
	}

	if claims == nil {
		claims = make(Claims)
	}
	return claims, nil
}

func saveClaims(dir string, claims Claims) error {
	claimsPath := filepath.Join(dir, "claims.json")
	data, err := json.MarshalIndent(claims, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal claims: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(claimsPath, data, 0644)
}
