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
	HasChanges  bool
	Error       string
	Attempts    int
}

type AgentStatus struct {
	TestStatus    string // "pass", "fail", "unknown"
	HasUncommitted bool
	ClaudeRunning  bool
}

// RunUntilDone runs the agent task loop until complete or max attempts reached.
func RunUntilDone(name string, task string, maxAttempts int) (*TaskResult, error) {
	result := &TaskResult{}

	if maxAttempts == 0 {
		maxAttempts = 10
	}

	var repoURL string
	if agent, err := loadAgent(name); err == nil && agent.Repo != "" {
		repoURL = agent.Repo
		if _, err := coordination.Init(repoURL); err != nil {
			repoURL = ""
		}
	}

	loopStart := time.Now()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result.Attempts = attempt
		fmt.Printf("\n🔄 Attempt %d/%d\n", attempt, maxAttempts)

		if repoURL != "" {
			coordination.UpdateAgentState(repoURL, name, "working", "")
		}

		if repoURL != "" {
			if needsRebase, _ := coordination.HasRebaseNeeded(repoURL, name, loopStart); needsRebase {
				fmt.Printf("⚠️  Rebase needed signal detected\n")
				task = task + "\n\nIMPORTANT: Another agent has pushed changes. Run 'git pull --rebase' before continuing."
			}
		}

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

		fmt.Printf("🤖 Running agent...\n")
		if err := runTask(name, prompt); err != nil {
			fmt.Printf("⚠️  Agent error: %v\n", err)
		}

		time.Sleep(2 * time.Second)

		status := getStatus(name)
		fmt.Printf("📊 Status: tests=%s uncommitted=%v\n", status.TestStatus, status.HasUncommitted)

		result.TestsPassed = status.TestStatus == "pass"
		result.HasChanges = status.HasUncommitted

		if result.TestsPassed && !result.HasChanges {
			result.Completed = true
			fmt.Printf("✅ Task completed!\n")

			if repoURL != "" {
				coordination.UpdateAgentState(repoURL, name, "done", "")
				coordination.ReleaseAllForAgent(repoURL, name)
			}

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

		fmt.Printf("⏳ Not done yet, continuing...\n")
		time.Sleep(3 * time.Second)
	}

	if repoURL != "" {
		coordination.UpdateAgentState(repoURL, name, "blocked", "")
	}

	result.Error = "max attempts reached"
	return result, fmt.Errorf("task not completed after %d attempts", maxAttempts)
}

// CheckCompletion checks if an agent's task appears complete.
func CheckCompletion(name string) AgentStatus {
	return getStatus(name)
}

func getStatus(name string) AgentStatus {
	status := AgentStatus{TestStatus: "unknown"}

	agent, err := loadAgent(name)
	if err != nil {
		return status
	}

	dir := agent.CloneDir

	// Check for uncommitted changes
	out, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	status.HasUncommitted = len(strings.TrimSpace(string(out))) > 0

	// Check if tests pass
	testCmds := []struct {
		check string
		run   string
	}{
		{
			check: fmt.Sprintf("test -f %s/vendor/bin/pest", dir),
			run:   fmt.Sprintf("cd %s && vendor/bin/pest --no-coverage 2>&1; echo EXIT_CODE:$?", dir),
		},
		{
			check: fmt.Sprintf("test -f %s/package.json", dir),
			run:   fmt.Sprintf("cd %s && npm test 2>&1; echo EXIT_CODE:$?", dir),
		},
		{
			check: fmt.Sprintf("test -f %s/go.mod", dir),
			run:   fmt.Sprintf("cd %s && go test ./... 2>&1; echo EXIT_CODE:$?", dir),
		},
		{
			check: fmt.Sprintf("test -f %s/pytest.ini -o -f %s/pyproject.toml", dir, dir),
			run:   fmt.Sprintf("cd %s && pytest 2>&1; echo EXIT_CODE:$?", dir),
		},
	}

	for _, tc := range testCmds {
		if err := exec.Command("sh", "-c", tc.check).Run(); err != nil {
			continue
		}
		out, _ := exec.Command("sh", "-c", tc.run).Output()
		if strings.Contains(string(out), "EXIT_CODE:0") {
			status.TestStatus = "pass"
		} else {
			status.TestStatus = "fail"
		}
		break
	}

	// Check if run-task / claude process is active in this clone dir
	out, _ = exec.Command("sh", "-c",
		fmt.Sprintf("ps aux 2>/dev/null | grep -v grep | grep '%s' || true", dir)).Output()
	status.ClaudeRunning = len(strings.TrimSpace(string(out))) > 0

	return status
}

// runTask calls run-task in the agent's clone directory.
// run-task must be available in PATH — each environment provides its own implementation.
func runTask(name string, prompt string) error {
	agent, err := loadAgent(name)
	if err != nil {
		return err
	}

	escaped := strings.ReplaceAll(prompt, "'", "'\\''")
	log := logFile(name)

	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("cd %s && run-task '%s' 2>&1 | tee -a %s", agent.CloneDir, escaped, log))

	output, err := cmd.CombinedOutput()
	if len(output) > 500 {
		fmt.Printf("📝 Output (truncated): %s...\n", string(output[:500]))
	} else if len(output) > 0 {
		fmt.Printf("📝 Output: %s\n", string(output))
	}

	return err
}
