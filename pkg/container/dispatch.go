package container

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const DispatchPreamble = `Do ALL work yourself, directly in this session. Do NOT delegate to subagents or task agents — they do not receive this spec.

Branch discipline:
- Create a feature branch for this work.
- Push the branch immediately upon creation.
- Commit and push after every working increment.
- Never include Co-Authored-By lines.

PR instruction: When the task originates from a GitHub issue, open a PR to the base branch whose body includes "Closes #N".
`

// ValidateDispatchArgs enforces exactly one intent source. It only reports;
// the exit decision belongs to the caller (main), never to a library function.
func ValidateDispatchArgs(issue, intent, intentFile string) (int, string) {
	count := 0
	for _, v := range []string{issue, intent, intentFile} {
		if v != "" {
			count++
		}
	}
	if count == 0 {
		return 64, "dispatch requires exactly one of --issue, --intent, or --intent-file"
	}
	if count > 1 {
		return 64, "dispatch accepts only one intent source: --issue OR --intent OR --intent-file"
	}
	return 0, ""
}

func DefaultModel(m string) string {
	if m == "" {
		return "cloud-smart"
	}
	return m
}

// IntentSource labels which source produced the intent, for the status line.
func IntentSource(issue, intent, intentFile string) string {
	switch {
	case issue != "":
		return "issue #" + issue
	case intentFile != "":
		return "intent-file"
	default:
		return "inline"
	}
}

// ComposeIntent builds the full worker prompt. Pure function (issueJSON is the
// raw `gh issue view --json title,body` output, fetched by the caller) so the
// composition the spec requires tests for is testable without podman/gh.
func ComposeIntent(issue, intent, intentFile, ownerRepo, issueJSON, fileContent string) string {
	switch {
	case issue != "":
		return DispatchPreamble + "\nYou are working on GitHub issue #" + issue + " for " + ownerRepo + ": " + issueJSON
	case intent != "":
		return DispatchPreamble + "\n" + intent
	case intentFile != "":
		return DispatchPreamble + "\n" + fileContent
	default:
		return DispatchPreamble
	}
}

func getHostGitIdentity() (string, string, error) {
	nameOut, err := exec.Command("git", "config", "--get", "user.name").Output()
	if err != nil {
		return "", "", fmt.Errorf("host git user.name not configured (fatal for dispatch identity)")
	}
	emailOut, err := exec.Command("git", "config", "--get", "user.email").Output()
	if err != nil {
		return "", "", fmt.Errorf("host git user.email not configured (fatal for dispatch identity)")
	}
	return strings.TrimSpace(string(nameOut)), strings.TrimSpace(string(emailOut)), nil
}

func ownerRepoOf(repo string) string {
	if strings.HasPrefix(repo, "https://") {
		return strings.TrimSuffix(strings.TrimPrefix(repo, "https://github.com/"), ".git")
	}
	return repo
}

// run wraps podman/gh/git steps so failures surface with context instead of
// vanishing. AGENT_LLM_KEY is NOT set here — Spawn already injects it as
// container env, which podman exec inherits (a .bashrc echo would be both
// redundant and a shell-injection vector).
func run(step string, args ...string) error {
	if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v: %s", step, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Dispatch(name, repo string, issue, intent, intentFile, model, branch, image string) error {
	if code, msg := ValidateDispatchArgs(issue, intent, intentFile); code != 0 {
		return fmt.Errorf("%s", msg)
	}
	model = DefaultModel(model)
	if image == "" {
		image = DefaultImage
	}

	// Resolve the git identity BEFORE spawning, so a missing identity fails
	// without leaving an orphaned container running.
	gitName, gitEmail, err := getHostGitIdentity()
	if err != nil {
		return err
	}

	if _, err := Spawn(name, repo, branch, image); err != nil {
		return err
	}
	// From here, any error must reap the container so the caller isn't left
	// with a half-provisioned worker.
	fail := func(e error) error {
		Kill(name)
		return e
	}

	ownerRepo := ownerRepoOf(repo)
	if err := run("gh clone", "podman", "exec", name, "gh", "repo", "clone", ownerRepo, "/home/agent/workspace/repo"); err != nil {
		return fail(err)
	}
	if err := run("gh auth setup-git", "podman", "exec", name, "gh", "auth", "setup-git"); err != nil {
		return fail(err)
	}
	if branch != "" {
		if err := run("checkout", "podman", "exec", name, "git", "-C", "/home/agent/workspace/repo", "checkout", branch); err != nil {
			return fail(err)
		}
	}
	if err := run("git user.name", "podman", "exec", name, "git", "-C", "/home/agent/workspace/repo", "config", "user.name", gitName); err != nil {
		return fail(err)
	}
	if err := run("git user.email", "podman", "exec", name, "git", "-C", "/home/agent/workspace/repo", "config", "user.email", gitEmail); err != nil {
		return fail(err)
	}

	var issueJSON, fileContent string
	if issue != "" {
		out, err := exec.Command("gh", "issue", "view", issue, "-R", ownerRepo, "--json", "title,body").Output()
		if err != nil {
			return fail(fmt.Errorf("gh issue view %s: %v", issue, err))
		}
		issueJSON = string(out)
	} else if intentFile != "" {
		data, err := os.ReadFile(intentFile)
		if err != nil {
			return fail(fmt.Errorf("read intent file: %v", err))
		}
		fileContent = string(data)
	}
	fullIntent := ComposeIntent(issue, intent, intentFile, ownerRepo, issueJSON, fileContent)

	// Private temp file (0600 via CreateTemp) instead of a predictable
	// world-readable /tmp path — intent can contain sensitive task detail.
	tmp, err := os.CreateTemp("", "agentctl-intent-*.txt")
	if err != nil {
		return fail(fmt.Errorf("create intent temp: %v", err))
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(fullIntent); err != nil {
		tmp.Close()
		return fail(fmt.Errorf("write intent temp: %v", err))
	}
	tmp.Close()
	if err := run("cp intent", "podman", "cp", tmp.Name(), name+":/home/agent/intent.txt"); err != nil {
		return fail(err)
	}

	if err := run("launch", "podman", "exec", "-d", "-w", "/home/agent/workspace/repo",
		"-e", "AGENT_LLM_MODEL="+model, name,
		"sh", "-c", "run-task \"$(cat /home/agent/intent.txt)\" > /home/agent/task.log 2>&1"); err != nil {
		return fail(err)
	}

	fmt.Printf("dispatched: %s\nmodel: %s   repo: %s   intent: %s\nfollow:  agentctl logs %s   (tails /home/agent/task.log)\nstatus:  agentctl status %s\n",
		name, model, repo, IntentSource(issue, intent, intentFile), name, name)
	return nil
}
