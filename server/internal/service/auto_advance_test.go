package service

import "testing"

func TestAutoAdvanceOnComplete_DefaultOnDisableWith0(t *testing.T) {
	// Default (unset) is ON — the self-healing advance is the desired behavior.
	t.Setenv("MULTICA_AUTO_ADVANCE", "")
	if !autoAdvanceOnComplete() {
		t.Fatal("expected auto-advance ON by default")
	}
	// Explicit opt-out.
	t.Setenv("MULTICA_AUTO_ADVANCE", "0")
	if autoAdvanceOnComplete() {
		t.Fatal("MULTICA_AUTO_ADVANCE=0 must disable auto-advance")
	}
	// Any other value leaves it on.
	t.Setenv("MULTICA_AUTO_ADVANCE", "1")
	if !autoAdvanceOnComplete() {
		t.Fatal("MULTICA_AUTO_ADVANCE=1 should keep auto-advance on")
	}
}

func TestIsTerminalIssueStatus(t *testing.T) {
	terminal := []string{"done", "cancelled"}
	nonTerminal := []string{"backlog", "todo", "in_progress", "in_review", "blocked", ""}
	for _, s := range terminal {
		if !isTerminalIssueStatus(s) {
			t.Errorf("%q should be terminal", s)
		}
	}
	for _, s := range nonTerminal {
		if isTerminalIssueStatus(s) {
			t.Errorf("%q should NOT be terminal (epic-with-this-child must block advance)", s)
		}
	}
}
