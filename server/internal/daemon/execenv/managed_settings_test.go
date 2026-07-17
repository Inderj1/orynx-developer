package execenv

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedSettingsDir(t *testing.T) {
	if got := managedSettingsDir("claude"); got != ".claude" {
		t.Fatalf("claude: got %q, want .claude", got)
	}
	for _, p := range []string{"codex", "opencode", "cursor", "hermes", ""} {
		if got := managedSettingsDir(p); got != "" {
			t.Fatalf("%s: expected unsupported (empty), got %q", p, got)
		}
	}
}

func TestManagedSettingsJSONValid(t *testing.T) {
	s := managedSettingsJSON("/w/.claude/hooks/bash-guard.py")
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	for _, want := range []string{"PreToolUse", `"Bash"`, "bash-guard.py"} {
		if !strings.Contains(s, want) {
			t.Fatalf("settings.json missing %q:\n%s", want, s)
		}
	}
}

func TestWriteManagedSettings_DisabledIsNoop(t *testing.T) {
	t.Setenv("MULTICA_AGENT_BASH_GUARD", "") // disabled
	dir := t.TempDir()
	if err := writeManagedSettings(dir, "claude", &sidecarManifest{}); err != nil {
		t.Fatalf("writeManagedSettings: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("expected no settings.json when guard disabled")
	}
}

func TestWriteManagedSettings_EnabledWritesFiles(t *testing.T) {
	t.Setenv("MULTICA_AGENT_BASH_GUARD", "1")
	dir := t.TempDir()
	if err := writeManagedSettings(dir, "claude", &sidecarManifest{}); err != nil {
		t.Fatalf("writeManagedSettings: %v", err)
	}
	settings := filepath.Join(dir, ".claude", "settings.json")
	guard := filepath.Join(dir, ".claude", "hooks", "bash-guard.py")
	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("settings.json invalid: %v", err)
	}
	if !strings.Contains(string(raw), guard) {
		t.Fatalf("settings.json does not reference the guard path %q", guard)
	}
	if _, err := os.Stat(guard); err != nil {
		t.Fatalf("guard script missing: %v", err)
	}
	// Unsupported provider is a no-op even when enabled.
	dir2 := t.TempDir()
	if err := writeManagedSettings(dir2, "codex", &sidecarManifest{}); err != nil {
		t.Fatalf("codex: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir2, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("expected no settings.json for unsupported provider")
	}
}

// TestBashGuardScriptBehavior runs the actual guard against sample payloads to
// prove the regexes deny catastrophic commands and allow ordinary ones. Skipped
// when python3 is unavailable (the guard is invoked as `python3 bash-guard.py`).
func TestBashGuardScriptBehavior(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	guard := filepath.Join(dir, "bash-guard.py")
	if err := os.WriteFile(guard, []byte(bashGuardScript), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(cmd string) bool { // returns true if DENIED
		payload, _ := json.Marshal(map[string]any{
			"tool_name":  "Bash",
			"tool_input": map[string]any{"command": cmd},
		})
		c := exec.Command(py, guard)
		c.Stdin = strings.NewReader(string(payload))
		out, err := c.Output()
		if err != nil {
			t.Fatalf("guard exec failed for %q: %v", cmd, err)
		}
		return strings.Contains(string(out), `"deny"`)
	}

	denied := []string{
		"rm -rf /",
		"rm -rf /*",
		"sudo rm -rf --no-preserve-root /",
		":(){ :|:& };:",
		"mkfs.ext4 /dev/sda1",
		"dd if=/dev/zero of=/dev/sda bs=1M",
		"docker run --rm -v /:/host alpine sh",
		"chmod -R 777 /",
	}
	for _, c := range denied {
		if !run(c) {
			t.Errorf("expected DENY for dangerous command: %q", c)
		}
	}

	allowed := []string{
		"rm -rf ./build",
		"rm -rf node_modules",
		"docker compose up -d",
		"docker run --rm -v ./data:/data alpine sh",
		"git push origin main",
		"pnpm test && next build",
		"pytest -q",
		"dd if=/dev/zero of=./testfile bs=1M count=1",
	}
	for _, c := range allowed {
		if run(c) {
			t.Errorf("expected ALLOW for safe command: %q", c)
		}
	}
}
