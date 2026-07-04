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

func ValidateDispatchArgs(issue, intent, intentFile string) (int, string) {
	count := 0
	if issue != "" {
		count++
	}
	if intent != "" {
		count++
	}
	if intentFile != "" {
		count++
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

func Dispatch(name, repo string, issue, intent, intentFile, model, branch, image string) error {
	code, msg := ValidateDispatchArgs(issue, intent, intentFile)
	if code != 0 {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(code)
	}
	model = DefaultModel(model)
	if image == "" {
		image = DefaultImage
	}
	_, err := Spawn(name, repo, branch, image)
	if err != nil {
		return err
	}
	if llmKey := resolveLLMKey(); llmKey != "" {
		exec.Command("podman", "exec", name, "sh", "-c", fmt.Sprintf("echo 'export AGENT_LLM_KEY=%s' >> /home/agent/.bashrc", llmKey)).Run()
	}
	exec.Command("podman", "exec", name, "sh", "-c", fmt.Sprintf("echo 'export AGENT_LLM_MODEL=%s' >> /home/agent/.bashrc", model)).Run()

	ownerRepo := repo
	if strings.HasPrefix(repo, "https://") {
		ownerRepo = strings.TrimSuffix(strings.TrimPrefix(repo, "https://github.com/"), ".git")
	}
	exec.Command("podman", "exec", name, "gh", "repo", "clone", ownerRepo, "/home/agent/workspace/repo").Run()
	exec.Command("podman", "exec", name, "gh", "auth", "setup-git").Run()
	if branch != "" {
		exec.Command("podman", "exec", name, "git", "-C", "/home/agent/workspace/repo", "checkout", branch).Run()
	}

	gitName, gitEmail, err := getHostGitIdentity()
	if err != nil {
		return err
	}
	exec.Command("podman", "exec", name, "git", "-C", "/home/agent/workspace/repo", "config", "user.name", gitName).Run()
	exec.Command("podman", "exec", name, "git", "-C", "/home/agent/workspace/repo", "config", "user.email", gitEmail).Run()

	var fullIntent string
	if issue != "" {
		out, _ := exec.Command("gh", "issue", "view", issue, "-R", ownerRepo, "--json", "title,body").Output()
		fullIntent = DispatchPreamble + "\nYou are working on GitHub issue #" + issue + " for " + ownerRepo + ": " + string(out)
	} else if intent != "" {
		fullIntent = DispatchPreamble + "\n" + intent
	} else if intentFile != "" {
		data, _ := os.ReadFile(intentFile)
		fullIntent = DispatchPreamble + "\n" + string(data)
	}
	tmp := "/tmp/intent." + name + ".txt"
	os.WriteFile(tmp, []byte(fullIntent), 0644)
	exec.Command("podman", "cp", tmp, name+":/home/agent/intent.txt").Run()
	os.Remove(tmp)

	exec.Command("podman", "exec", "-d", "-w", "/home/agent/workspace/repo", name,
		"sh", "-c", "run-task \"$(cat /home/agent/intent.txt)\" > /home/agent/task.log 2>&1").Run()

	fmt.Printf("dispatched: %s\nmodel: %s   repo: %s   intent: %s\nfollow:  agentctl logs %s   (tails /home/agent/task.log)\nstatus:  agentctl status %s\n",
		name, model, repo, map[bool]string{true: "issue #" + issue, false: "inline"}[issue != ""], name, name)
	return nil
}
