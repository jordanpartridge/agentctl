package container

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/jordanpartridge/agentctl/pkg/coordination"
)

type TaskResult struct {
	Completed   bool
	TestsPassed bool
	LintPassed  bool
	HasChanges  bool
	Error       string
	Attempts    int
}

type AgentStatus struct {
	TestStatus     string // "pass", "fail", "unknown"
	HasUncommitted bool
	ClaudeRunning  bool
}

// RunUntilDone keeps the agent working until the task is complete
// This implements the "Ralph Wiggum" pattern - persistent retry until success.
// When a repoURL is available (via agent metadata), it integrates with the
// coordination bus to update state and check for rebase_needed signals.
func RunUntilDone(name string, task string, maxAttempts int) (*TaskResult, error) {
	result := &TaskResult{}

	if maxAttempts == 0 {
		maxAttempts = 10 // default
	}

	// Look up agent metadata for coordination integration
	var repoURL string
	if agent, err := loadAgent(name); err == nil && agent.Repo != "" {
		repoURL = agent.Repo
		// Initialize coordination directory
		if _, err := coordination.Init(repoURL); err != nil {
			fmt.Printf("⚠️  Coordination init failed (continuing without): %v\n", err)
			repoURL = "" // disable coordination
		}
	}

	loopStart := time.Now()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result.Attempts = attempt
		fmt.Printf("\n🔄 Attempt %d/%d\n", attempt, maxAttempts)

		// Update coordination state
		if repoURL != "" {
			coordination.UpdateAgentState(repoURL, name, "working", "")
		}

		// Check for rebase_needed signals from other agents
		if repoURL != "" {
			if needsRebase, _ := coordination.HasRebaseNeeded(repoURL, name, loopStart); needsRebase {
				fmt.Printf("⚠️  Rebase needed signal detected, adding to prompt\n")
				task = task + "\n\nIMPORTANT: Another agent has pushed changes. Run 'git pull --rebase' before continuing."
			}
		}

		// Build the prompt - include context from previous attempts
		prompt := task
		if attempt > 1 {
			status := getStatus(name)
			prompt = fmt.Sprintf(`Continue working. Previous status:
- Tests: %s
- Uncommitted changes: %v

Original task: %s

Keep going until tests pass and all changes are committed.`,
				status.TestStatus, status.HasUncommitted, task)
		}

		// Run Claude
		fmt.Printf("🤖 Running Claude...\n")
		err := runClaude(name, prompt)
		if err != nil {
			fmt.Printf("⚠️  Claude error: %v\n", err)
		}

		// Wait a moment for things to settle
		time.Sleep(2 * time.Second)

		// Check if done
		status := getStatus(name)
		fmt.Printf("📊 Status: tests=%s uncommitted=%v\n", status.TestStatus, status.HasUncommitted)

		result.TestsPassed = status.TestStatus == "pass"
		result.HasChanges = status.HasUncommitted

		// Done if tests pass and no uncommitted changes
		if result.TestsPassed && !result.HasChanges {
			result.Completed = true
			fmt.Printf("✅ Task completed!\n")

			// Update coordination state to done and release all claims
			if repoURL != "" {
				coordination.UpdateAgentState(repoURL, name, "done", "")
				coordination.ReleaseAllForAgent(repoURL, name)
			}

			// Save completion history for eventual cleanup
			SaveHistory(&AgentHistory{
				Name:        name,
				Repo:        repoURL,
				Created:     loopStart,
				CompletedAt: time.Now(),
				Result:      "success",
				Attempts:    attempt,
			})

			return result, nil
		}

		// Not done, loop continues
		fmt.Printf("⏳ Not done yet, continuing...\n")
		time.Sleep(3 * time.Second)
	}

	// Update coordination state on failure
	if repoURL != "" {
		coordination.UpdateAgentState(repoURL, name, "blocked", "")
	}

	result.Error = "max attempts reached"
	return result, fmt.Errorf("task not completed after %d attempts", maxAttempts)
}

// CheckCompletion checks if an agent's task appears complete
func CheckCompletion(name string) AgentStatus {
	return getStatus(name)
}

func getStatus(name string) AgentStatus {
	status := AgentStatus{TestStatus: "unknown"}

	// Check for uncommitted changes
	out, _ := exec.Command("podman", "exec", name, "sh", "-c",
		"cd /home/agent/workspace/repo && git status --porcelain 2>/dev/null").Output()
	status.HasUncommitted = len(strings.TrimSpace(string(out))) > 0

	// Check if tests pass (try common test runners)
	// Use exit code for reliable pass/fail detection
	testCmds := []struct {
		check string // command to check if test runner exists
		run   string // command to run tests
	}{
		{
			check: "cd /home/agent/workspace/repo && test -f vendor/bin/pest",
			run:   "cd /home/agent/workspace/repo && vendor/bin/pest --no-coverage 2>&1; echo EXIT_CODE:$?",
		},
		{
			check: "cd /home/agent/workspace/repo && test -f package.json",
			run:   "cd /home/agent/workspace/repo && npm test 2>&1; echo EXIT_CODE:$?",
		},
		{
			check: "cd /home/agent/workspace/repo && test -f go.mod",
			run:   "cd /home/agent/workspace/repo && go test ./... 2>&1; echo EXIT_CODE:$?",
		},
		{
			check: "cd /home/agent/workspace/repo && test -f pytest.ini -o -f pyproject.toml",
			run:   "cd /home/agent/workspace/repo && pytest 2>&1; echo EXIT_CODE:$?",
		},
		{
			check: "cd /home/agent/workspace/repo && test -f Cargo.toml",
			run:   "cd /home/agent/workspace/repo && cargo test 2>&1; echo EXIT_CODE:$?",
		},
	}

	for _, tc := range testCmds {
		// Check if test runner exists
		if err := exec.Command("podman", "exec", name, "sh", "-c", tc.check).Run(); err != nil {
			continue
		}
		// Run tests and check exit code
		out, _ := exec.Command("podman", "exec", name, "sh", "-c", tc.run).Output()
		output := string(out)
		if strings.Contains(output, "EXIT_CODE:0") {
			status.TestStatus = "pass"
		} else {
			status.TestStatus = "fail"
		}
		break
	}

	// Check if Claude is running
	out, _ = exec.Command("podman", "exec", name, "sh", "-c",
		"ps aux 2>/dev/null | grep -v grep | grep claude || true").Output()
	status.ClaudeRunning = len(strings.TrimSpace(string(out))) > 0

	return status
}

func runClaude(name string, prompt string) error {
	// Escape the prompt for shell
	escaped := strings.ReplaceAll(prompt, "'", "'\\''")

	cmd := exec.Command("podman", "exec", name, "sh", "-c",
		fmt.Sprintf("cd /home/agent/workspace/repo && claude --dangerously-skip-permissions -p '%s' 2>&1 | tee -a /home/agent/claude.log", escaped))

	output, err := cmd.CombinedOutput()
	if len(output) > 500 {
		fmt.Printf("📝 Output (truncated): %s...\n", string(output[:500]))
	} else if len(output) > 0 {
		fmt.Printf("📝 Output: %s\n", string(output))
	}

	return err
}
