package handler

import (
	"testing"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestAllChildrenTerminal(t *testing.T) {
	mk := func(statuses ...string) []db.Issue {
		out := make([]db.Issue, len(statuses))
		for i, s := range statuses {
			out[i] = db.Issue{Status: s}
		}
		return out
	}
	cases := []struct {
		name string
		in   []db.Issue
		want bool
	}{
		{"empty is not an epic", mk(), false},
		{"all done", mk("done", "done"), true},
		{"done + cancelled", mk("done", "cancelled"), true},
		{"one still open", mk("done", "in_progress"), false},
		{"one in_review is not terminal", mk("done", "in_review"), false},
		{"single done", mk("done"), true},
	}
	for _, c := range cases {
		if got := allChildrenTerminal(c.in); got != c.want {
			t.Errorf("%s: allChildrenTerminal = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAutoCloseEpicsEnabled_DefaultOnDisableWith0(t *testing.T) {
	t.Setenv("MULTICA_AUTO_ADVANCE", "")
	if !autoCloseEpicsEnabled() {
		t.Fatal("expected epic auto-close ON by default")
	}
	t.Setenv("MULTICA_AUTO_ADVANCE", "0")
	if autoCloseEpicsEnabled() {
		t.Fatal("MULTICA_AUTO_ADVANCE=0 must disable epic auto-close")
	}
}
