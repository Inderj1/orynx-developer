package handler

import "testing"

func TestComputeActionKey(t *testing.T) {
	issue := "11111111-1111-1111-1111-111111111111"

	// Deterministic + whitespace-normalized: formatting differences across a
	// fresh run still resolve to the same approval.
	a := ComputeActionKey(issue, "git push origin main")
	b := ComputeActionKey(issue, "git   push    origin main")
	c := ComputeActionKey(issue, "  git push origin main  ")
	if a != b || a != c {
		t.Fatalf("whitespace variants must produce the same key: %s / %s / %s", a, b, c)
	}

	// Different command => different key.
	if ComputeActionKey(issue, "git push origin main") == ComputeActionKey(issue, "git push origin dev") {
		t.Fatal("different commands must produce different keys")
	}
	// Same command, different issue => different key (approvals are per-issue).
	other := "22222222-2222-2222-2222-222222222222"
	if ComputeActionKey(issue, "git push") == ComputeActionKey(other, "git push") {
		t.Fatal("same command on different issues must not share an approval key")
	}
	// Stable hex length (sha256).
	if len(a) != 64 {
		t.Fatalf("action key should be a 64-char sha256 hex, got %d", len(a))
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 80); got != "short" {
		t.Fatalf("under-limit should pass through, got %q", got)
	}
	long := ""
	for i := 0; i < 100; i++ {
		long += "x"
	}
	got := truncate(long, 80)
	if len([]rune(got)) != 81 { // 80 chars + the ellipsis rune
		t.Fatalf("expected 80 chars + ellipsis, got %d runes", len([]rune(got)))
	}
}
