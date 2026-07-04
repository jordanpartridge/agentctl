package container

import (
	"strings"
	"testing"
)

func TestDispatchPreambleContainsNoSubagent(t *testing.T) {
	if !strings.Contains(DispatchPreamble, "Do NOT delegate to subagents or task agents") {
		t.Errorf("preamble missing no-subagent sentence")
	}
}

func TestDispatchPreambleContainsPushEarly(t *testing.T) {
	if !strings.Contains(DispatchPreamble, "Push the branch immediately upon creation") {
		t.Errorf("preamble missing push-early discipline")
	}
}

func TestValidateDispatchArgs(t *testing.T) {
	if code, _ := ValidateDispatchArgs("", "", ""); code != 64 {
		t.Errorf("expected 64 for no source")
	}
	if code, _ := ValidateDispatchArgs("1", "text", ""); code != 64 {
		t.Errorf("expected 64 for duplicate sources")
	}
	if code, _ := ValidateDispatchArgs("1", "", ""); code != 0 {
		t.Errorf("expected 0 for valid --issue")
	}
}

func TestDefaultModel(t *testing.T) {
	if DefaultModel("") != "cloud-smart" {
		t.Errorf("default model should be cloud-smart")
	}
	if DefaultModel("local-fast") != "local-fast" {
		t.Errorf("explicit model preserved")
	}
}

func TestIntentSource(t *testing.T) {
	cases := []struct{ issue, intent, file, want string }{
		{"5", "", "", "issue #5"},
		{"", "do a thing", "", "inline"},
		{"", "", "/path/spec.md", "intent-file"},
	}
	for _, c := range cases {
		if got := IntentSource(c.issue, c.intent, c.file); got != c.want {
			t.Errorf("IntentSource(%q,%q,%q) = %q, want %q", c.issue, c.intent, c.file, got, c.want)
		}
	}
}

func TestComposeIntentIssue(t *testing.T) {
	got := ComposeIntent("7", "", "", "owner/repo", `{"title":"T","body":"B"}`, "")
	if !strings.HasPrefix(got, DispatchPreamble) {
		t.Errorf("composed intent must start with preamble")
	}
	if !strings.Contains(got, "issue #7 for owner/repo") {
		t.Errorf("issue intent missing issue reference: %q", got)
	}
	if !strings.Contains(got, `"title":"T"`) {
		t.Errorf("issue intent missing issue JSON")
	}
}

func TestComposeIntentInline(t *testing.T) {
	got := ComposeIntent("", "build the widget", "", "owner/repo", "", "")
	if !strings.Contains(got, "build the widget") {
		t.Errorf("inline intent missing text")
	}
	if strings.Contains(got, "issue #") {
		t.Errorf("inline intent should not reference an issue")
	}
}

func TestComposeIntentFile(t *testing.T) {
	got := ComposeIntent("", "", "/spec.md", "owner/repo", "", "FILE SPEC BODY")
	if !strings.Contains(got, "FILE SPEC BODY") {
		t.Errorf("file intent missing file content")
	}
}

func TestOwnerRepoOf(t *testing.T) {
	cases := map[string]string{
		"owner/repo":                          "owner/repo",
		"https://github.com/owner/repo":       "owner/repo",
		"https://github.com/owner/repo.git":   "owner/repo",
	}
	for in, want := range cases {
		if got := ownerRepoOf(in); got != want {
			t.Errorf("ownerRepoOf(%q) = %q, want %q", in, got, want)
		}
	}
}
