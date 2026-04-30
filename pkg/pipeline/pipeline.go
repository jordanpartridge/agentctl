package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Step is a single pipeline step.
type Step struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// Pipeline is the parsed pipeline.yml structure.
type Pipeline struct {
	Steps []Step `yaml:"steps"`
}

// Options controls pipeline execution.
type Options struct {
	DryRun   bool
	FromStep string
}

// defaultPipeline is the built-in pipeline used when no pipeline.yml is found.
var defaultPipeline = Pipeline{
	Steps: []Step{
		{Name: "setup", Run: "composer install --no-interaction --quiet"},
		{Name: "investigate", Run: `run-task "Read issue #$ISSUE. Map files. Write DESIGN.md."`},
		{Name: "implement", Run: `run-task "Implement per DESIGN.md. TDD. Commit when green."`},
		{Name: "pr", Run: `gh pr create --title "$ISSUE_TITLE" --body "Closes #$ISSUE" --base master`},
		{Name: "review", Run: "agentctl review $AGENTCTL_NAME"},
		{Name: "merge", Run: "gh pr merge --squash --auto $PR_NUMBER"},
	},
}

// Load reads a pipeline from the given path, or falls back to ~/.agentctl/pipeline.yml,
// or returns the built-in default pipeline.
func Load(repoPath string) (*Pipeline, error) {
	// 1. Try repo-local pipeline.yml
	repoPipeline := filepath.Join(repoPath, "pipeline.yml")
	if p, err := loadFile(repoPipeline); err == nil {
		return p, nil
	}

	// 2. Try ~/.agentctl/pipeline.yml
	home, err := os.UserHomeDir()
	if err == nil {
		defaultPath := filepath.Join(home, ".agentctl", "pipeline.yml")
		if p, err := loadFile(defaultPath); err == nil {
			return p, nil
		}
	}

	// 3. Built-in default
	p := defaultPipeline
	return &p, nil
}

func loadFile(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Pipeline
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &p, nil
}

// Run executes the pipeline for the given repo and issue.
func Run(repo, issue string, opts Options) error {
	repoName := repoBaseName(repo)
	cloneDir := filepath.Join("/tmp/agents", fmt.Sprintf("%s-%s", repoName, issue))

	// Clone if needed.
	if !opts.DryRun {
		if err := ensureClone(repo, cloneDir); err != nil {
			return fmt.Errorf("clone failed: %w", err)
		}
	}

	// Fetch issue title from gh.
	issueTitle := ""
	if !opts.DryRun {
		issueTitle = fetchIssueTitle(repo, issue)
	}

	// Build branch name.
	branch := fmt.Sprintf("feature/%s-%s", repoName, issue)

	// Checkout/create branch.
	if !opts.DryRun {
		if err := ensureBranch(cloneDir, branch); err != nil {
			return fmt.Errorf("branch setup failed: %w", err)
		}
	}

	// Load pipeline.
	p, err := Load(cloneDir)
	if err != nil {
		return fmt.Errorf("loading pipeline: %w", err)
	}

	// Find starting step index.
	startIdx := 0
	if opts.FromStep != "" {
		found := false
		for i, step := range p.Steps {
			if step.Name == opts.FromStep {
				startIdx = i
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("step %q not found in pipeline", opts.FromStep)
		}
	}

	prNumber := ""

	for i, step := range p.Steps {
		if i < startIdx {
			continue
		}

		// Inject current PR_NUMBER into env.
		env := buildEnv(repo, issue, issueTitle, cloneDir, branch, repoName, prNumber)

		if opts.DryRun {
			fmt.Printf("[dry-run] step %d: %s\n  run: %s\n", i+1, step.Name, step.Run)
			continue
		}

		fmt.Printf("▶ [%d/%d] %s\n", i+1, len(p.Steps), step.Name)

		if err := runStep(step, cloneDir, env); err != nil {
			return fmt.Errorf("step %q failed: %w", step.Name, err)
		}

		// After any step whose run contains "gh pr create", detect PR number.
		if strings.Contains(step.Run, "gh pr create") {
			prNumber = detectPRNumber(cloneDir, branch)
			if prNumber != "" {
				fmt.Printf("  detected PR #%s\n", prNumber)
			}
		}
	}

	if !opts.DryRun {
		fmt.Println("✅ Pipeline complete")
	}
	return nil
}

func runStep(step Step, cloneDir string, env []string) error {
	cmd := exec.Command("sh", "-c", step.Run)
	cmd.Dir = cloneDir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildEnv(repo, issue, issueTitle, cloneDir, branch, agentctlName, prNumber string) []string {
	base := os.Environ()
	extras := []string{
		"REPO=" + repo,
		"ISSUE=" + issue,
		"ISSUE_TITLE=" + issueTitle,
		"CLONE_DIR=" + cloneDir,
		"BRANCH=" + branch,
		"PR_NUMBER=" + prNumber,
		"AGENTCTL_NAME=" + agentctlName,
	}
	return append(base, extras...)
}

func ensureClone(repo, cloneDir string) error {
	if _, err := os.Stat(cloneDir); err == nil {
		// Already cloned — fetch latest.
		cmd := exec.Command("git", "-C", cloneDir, "fetch", "--quiet")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	if err := os.MkdirAll(filepath.Dir(cloneDir), 0o755); err != nil {
		return err
	}

	// Resolve full HTTPS URL if only owner/repo is given.
	repoURL := repo
	if !strings.Contains(repo, "://") && !strings.HasPrefix(repo, "git@") {
		repoURL = "https://github.com/" + repo
	}

	cmd := exec.Command("git", "clone", repoURL, cloneDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ensureBranch(cloneDir, branch string) error {
	// Check if branch already exists.
	check := exec.Command("git", "-C", cloneDir, "rev-parse", "--verify", branch)
	if check.Run() == nil {
		// Branch exists — check it out.
		cmd := exec.Command("git", "-C", cloneDir, "checkout", branch)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	// Create and checkout new branch.
	cmd := exec.Command("git", "-C", cloneDir, "checkout", "-b", branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fetchIssueTitle(repo, issue string) string {
	// Resolve repo slug for gh (strip https://github.com/ prefix if present).
	slug := repo
	slug = strings.TrimPrefix(slug, "https://github.com/")
	slug = strings.TrimPrefix(slug, "http://github.com/")
	slug = strings.TrimPrefix(slug, "git@github.com:")
	slug = strings.TrimSuffix(slug, ".git")

	out, err := exec.Command("gh", "issue", "view", issue, "--repo", slug, "--json", "title", "-q", ".title").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectPRNumber(cloneDir, branch string) string {
	out, err := exec.Command("gh", "pr", "list", "--head", branch, "--json", "number", "-q", ".[0].number").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func repoBaseName(repo string) string {
	// Strip common URL prefixes/suffixes.
	r := repo
	r = strings.TrimPrefix(r, "https://github.com/")
	r = strings.TrimPrefix(r, "http://github.com/")
	r = strings.TrimPrefix(r, "git@github.com:")
	r = strings.TrimSuffix(r, ".git")
	// Use last path component.
	parts := strings.Split(r, "/")
	return parts[len(parts)-1]
}
