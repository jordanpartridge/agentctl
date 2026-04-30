package review

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/jordanpartridge/agentctl/pkg/container"
)

// Result represents the outcome of a PR review.
type Result struct {
	Approved bool
	Feedback string
}

// prInfo holds the PR number and URL returned by gh.
type prInfo struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// lexiRequest is the payload sent to POST /api/prompt.
type lexiRequest struct {
	Message string `json:"message"`
}

// lexiResponse is the response from POST /api/prompt.
type lexiResponse struct {
	Response string `json:"response"`
	// Lexi may also return the reply under "message" depending on version.
	Message string `json:"message"`
}

// Review loads agent metadata, finds the open PR, asks Lexi to review it,
// and returns a Result indicating approval or changes requested.
func Review(name string) (*Result, error) {
	// 1. Load agent metadata.
	agent, err := container.LoadAgent(name)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	// 2. Resolve the GitHub repo slug from the full URL.
	repo := repoSlug(agent.Repo)

	// 3. Find the open PR for the agent's branch.
	fmt.Printf("🔍 Looking up open PR for %s on branch %s...\n", repo, agent.Branch)
	pr, err := findOpenPR(repo, agent.Branch)
	if err != nil {
		return nil, fmt.Errorf("could not find open PR: %w", err)
	}

	fmt.Printf("🔍 Reviewing PR #%d for agent %s...\n", pr.Number, name)

	// 4. Call Lexi.
	cfg := LoadConfig()
	if cfg.LexiToken == "" {
		return nil, fmt.Errorf("no Lexi token: set APP_KEY env var or lexi_token in ~/.agentctl/config.json")
	}

	message := fmt.Sprintf(
		"Review PR #%d in %s. Check it closes the original issue requirements. "+
			"Use FetchPullRequestDiff and SubmitPullRequestReview. "+
			"Approve if good, request changes if not. "+
			"Reply with just APPROVED or CHANGES_REQUESTED: <feedback>",
		pr.Number, repo,
	)

	fmt.Println("🤖 Asking Lexi to review...")
	reply, err := callLexi(cfg, message)
	if err != nil {
		return nil, fmt.Errorf("Lexi request failed: %w", err)
	}

	// 5. Parse Lexi's reply.
	return parseReply(reply), nil
}

// findOpenPR uses the gh CLI to locate the first open PR for the given branch.
func findOpenPR(repo, branch string) (*prInfo, error) {
	args := []string{
		"pr", "list",
		"--repo", repo,
		"--head", branch,
		"--state", "open",
		"--json", "number,url",
		"-q", ".[0]",
	}
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}

	text := strings.TrimSpace(string(out))
	if text == "" || text == "null" {
		return nil, fmt.Errorf("no open PR found for branch %q in %s", branch, repo)
	}

	var pr prInfo
	if err := json.Unmarshal([]byte(text), &pr); err != nil {
		return nil, fmt.Errorf("failed to parse PR info: %w", err)
	}
	if pr.Number == 0 {
		return nil, fmt.Errorf("no open PR found for branch %q in %s", branch, repo)
	}
	return &pr, nil
}

// callLexi POSTs a prompt to Lexi's /api/prompt endpoint and returns the reply text.
func callLexi(cfg Config, message string) (string, error) {
	payload, err := json.Marshal(lexiRequest{Message: message})
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(cfg.LexiURL, "/") + "/api/prompt"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.LexiToken)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("Lexi returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var lr lexiResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		// Lexi might just return a plain string — treat the raw body as the reply.
		return strings.TrimSpace(string(body)), nil
	}

	// Prefer "response" field; fall back to "message".
	reply := lr.Response
	if reply == "" {
		reply = lr.Message
	}
	if reply == "" {
		reply = strings.TrimSpace(string(body))
	}
	return reply, nil
}

// parseReply interprets Lexi's response and returns a Result.
func parseReply(reply string) *Result {
	upper := strings.ToUpper(reply)
	if strings.Contains(upper, "APPROVED") && !strings.Contains(upper, "CHANGES_REQUESTED") {
		return &Result{Approved: true}
	}

	// Extract feedback after "CHANGES_REQUESTED:"
	feedback := reply
	if idx := strings.Index(upper, "CHANGES_REQUESTED:"); idx >= 0 {
		feedback = strings.TrimSpace(reply[idx+len("CHANGES_REQUESTED:"):])
	} else if idx := strings.Index(upper, "CHANGES_REQUESTED"); idx >= 0 {
		feedback = strings.TrimSpace(reply[idx+len("CHANGES_REQUESTED"):])
		feedback = strings.TrimLeft(feedback, ": ")
	}

	return &Result{Approved: false, Feedback: feedback}
}

// repoSlug converts a full GitHub URL to owner/repo format.
// If already in that format it passes through unchanged.
func repoSlug(repo string) string {
	// Strip trailing .git
	repo = strings.TrimSuffix(repo, ".git")

	// Handle https://github.com/owner/repo
	for _, prefix := range []string{"https://github.com/", "git@github.com:"} {
		if strings.HasPrefix(repo, prefix) {
			return strings.TrimPrefix(repo, prefix)
		}
	}
	return repo
}
