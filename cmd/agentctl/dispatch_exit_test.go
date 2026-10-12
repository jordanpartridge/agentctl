package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// AGENTCTL_EXIT_HELPER re-execs this test binary as agentctl so the process
// exit code is what a caller observes. "fail" is a podman run that exits 126.
// "ok" is a podman run that starts.
func TestMain(m *testing.M) {
	switch os.Getenv("AGENTCTL_EXIT_HELPER") {
	case "fail":
		os.Args = []string{"agentctl", "dispatch", "fail-agent", "owner/repo", "--intent", "do the thing"}
		main()
		os.Exit(0)
	case "ok":
		os.Args = []string{"agentctl", "dispatch", "ok-agent", "owner/repo", "--intent", "do the thing"}
		main()
		os.Exit(0)
	default:
		os.Exit(m.Run())
	}
}

func TestDispatchSpawnFailureExitsNonZero(t *testing.T) {
	stdout, stderr, log, code := execDispatch(t, "fail", "126", "created")
	if code == 0 {
		t.Fatalf("failed spawn exited 0\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "spawn failed") {
		t.Fatalf("stderr = %q, want spawn failed", stderr)
	}
	if !strings.Contains(log, "rm -f fail-agent") {
		t.Fatalf("created container was not reaped\nlog:\n%s", log)
	}
}

func TestDispatchSpawnSuccessExitsZero(t *testing.T) {
	stdout, stderr, log, code := execDispatch(t, "ok", "0", "running")
	if code != 0 {
		t.Fatalf("successful spawn exited %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "dispatched: ok-agent") {
		t.Fatalf("stdout = %q, want dispatched handle", stdout)
	}
	if strings.Contains(log, "rm -f ok-agent") {
		t.Fatalf("successful spawn reaped the container\nlog:\n%s", log)
	}
}

func execDispatch(t *testing.T, helper, runExit, inspectStatus string) (stdout, stderr, log string, code int) {
	t.Helper()
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(home, "podman.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$PODMAN_LOG"
if [ "$1" = "run" ]; then
  echo fakecid0123456789abcdef
  exit "$PODMAN_RUN_EXIT"
fi
if [ "$1" = "inspect" ]; then
  printf '%s\n' "$PODMAN_INSPECT_STATUS"
  exit 0
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	gitconfig := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[user]\n\tname = Test User\n\temail = test@example.com\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0])
	cmd.Dir = home
	env := make([]string, 0, len(os.Environ())+8)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "HOME=") ||
			strings.HasPrefix(kv, "GIT_CONFIG_GLOBAL=") || strings.HasPrefix(kv, "AGENTCTL_EXIT_HELPER=") ||
			strings.HasPrefix(kv, "PODMAN_LOG=") || strings.HasPrefix(kv, "PODMAN_RUN_EXIT=") ||
			strings.HasPrefix(kv, "PODMAN_INSPECT_STATUS=") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = append(env,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+home,
		"GIT_CONFIG_GLOBAL="+gitconfig,
		"GIT_CONFIG_NOSYSTEM=1",
		"AGENTCTL_EXIT_HELPER="+helper,
		"PODMAN_LOG="+logPath,
		"PODMAN_RUN_EXIT="+runExit,
		"PODMAN_INSPECT_STATUS="+inspectStatus,
	)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code = 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run dispatch helper: %v", err)
		}
		code = ee.ExitCode()
	}
	logged, _ := os.ReadFile(logPath)
	return outBuf.String(), errBuf.String(), string(logged), code
}
