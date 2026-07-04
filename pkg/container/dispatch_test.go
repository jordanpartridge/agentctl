package container

import "testing"

func TestDispatchPreambleContainsNoSubagent(t *testing.T) {
	if !contains(DispatchPreamble, "Do NOT delegate to subagents or task agents") {
		t.Errorf("preamble missing no-subagent sentence")
	}
}

func TestDispatchPreambleContainsPushEarly(t *testing.T) {
	if !contains(DispatchPreamble, "Push the branch immediately upon creation") {
		t.Errorf("preamble missing push-early discipline")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
