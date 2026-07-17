package execenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Managed Claude Code settings.json — a deterministic guardrail layer.
//
// The agent runs with --permission-mode bypassPermissions (every tool
// auto-approved), so the model's own judgement is the only thing standing
// between a prompt-injected instruction and a destructive shell command. A
// PreToolUse hook is the one gate that fires REGARDLESS of permission mode and
// can hard-deny a tool call, so we install a Bash guard that blocks a small set
// of catastrophic commands (rm -rf /, fork bombs, disk wipes, mounting host
// root into a container). It is intentionally narrow: it must never interfere
// with normal build/test/git work, only stop the handful of commands that are
// unrecoverable. It complements — does not replace — the execution sandbox
// (see docs/design/agent-sandbox.md); this is the in-process backstop.
//
// Gated by MULTICA_AGENT_BASH_GUARD=1 so merging changes no behavior until an
// operator opts in and validates on the box.

// bashGuardEnabled reports whether the managed Bash guardrail should be written.
func bashGuardEnabled() bool {
	return os.Getenv("MULTICA_AGENT_BASH_GUARD") == "1"
}

// managedSettingsDir returns the provider-relative config dir that holds
// settings.json, or "" for providers that do not use the Claude Code
// settings.json + hooks schema. Only Claude is wired today; forks that share
// the exact schema can be added here once verified.
func managedSettingsDir(provider string) string {
	switch provider {
	case "claude":
		return ".claude"
	default:
		return ""
	}
}

// bashGuardScript reads the PreToolUse hook payload on stdin and denies the
// Bash tool call when its command matches a catastrophic pattern. Fail-open on
// any parse/lookup error: a guardrail that breaks normal runs is worse than a
// narrow one, and the sandbox is the real containment boundary.
const bashGuardScript = `#!/usr/bin/env python3
import sys, json, re
# Catastrophic, unrecoverable commands only. Deliberately narrow so ordinary
# build/test/git/docker work is never blocked. The sandbox is the real
# boundary; this only stops the handful of one-shot foot-guns.
DANGER = [
    r"rm\s+-rf?\s+(--no-preserve-root\s+)?/(\s|$|\*)",   # rm -rf /  or  /*
    r":\(\)\s*\{\s*:\|:&\s*\}\s*;:",                       # classic fork bomb
    r"\bmkfs(\.\w+)?\b",                                    # format a filesystem
    r"\bdd\b[^\n]*\bof=/dev/(sd|nvme|xvd|vd)",              # overwrite a raw disk
    r">\s*/dev/(sd|nvme|xvd|vd)",                           # redirect onto a raw disk
    r"docker\s+run\b[^\n]*-v\s*/:(/|\s)",                   # mount host root into a container
    r"\bchmod\s+-R\s+0*777\s+/(\s|$)",                      # world-writable root
]
try:
    data = json.load(sys.stdin)
    cmd = (data.get("tool_input") or {}).get("command", "") or ""
except Exception:
    sys.exit(0)  # unparseable -> allow (fail-open)
for pat in DANGER:
    if re.search(pat, cmd):
        print(json.dumps({"hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": "Blocked by Orynx guardrail: command matches a catastrophic pattern and was denied before execution.",
        }}))
        sys.exit(0)
sys.exit(0)
`

// managedSettingsJSON returns the settings.json content wiring the PreToolUse
// Bash guard at the given absolute guard-script path.
func managedSettingsJSON(guardPath string) string {
	// Hand-rendered so the shape is obvious and stable in diffs/tests.
	return fmt.Sprintf(`{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          { "type": "command", "command": "python3 %s" }
        ]
      }
    ]
  }
}
`, guardPath)
}

// writeManagedSettings writes a managed .claude/settings.json plus the Bash
// guard script into workDir, when enabled and the provider supports the schema.
// No-op (nil) when disabled or unsupported. Pre-existing files are left alone
// (the operator owns them) — same policy as the rest of writeContextFiles.
func writeManagedSettings(workDir, provider string, manifest *sidecarManifest) error {
	if !bashGuardEnabled() {
		return nil
	}
	cfgDir := managedSettingsDir(provider)
	if cfgDir == "" {
		return nil
	}
	hooksDir := filepath.Join(workDir, cfgDir, "hooks")
	if err := recordMkdirAll(hooksDir, 0o755, manifest); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}
	guardPath := filepath.Join(hooksDir, "bash-guard.py")
	if err := recordWriteFile(guardPath, []byte(bashGuardScript), 0o755, manifest); err != nil {
		if !errors.Is(err, errPathPreExists) {
			return fmt.Errorf("write bash-guard.py: %w", err)
		}
	}
	settingsPath := filepath.Join(workDir, cfgDir, "settings.json")
	if err := recordWriteFile(settingsPath, []byte(managedSettingsJSON(guardPath)), 0o644, manifest); err != nil {
		if !errors.Is(err, errPathPreExists) {
			return fmt.Errorf("write settings.json: %w", err)
		}
	}
	return nil
}
