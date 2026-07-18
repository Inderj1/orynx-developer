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

// approvalGateEnabled reports whether the tier-2 irreversible-action approval
// gate should be wired into the hook (P0.2).
func approvalGateEnabled() bool {
	return os.Getenv("MULTICA_AGENT_APPROVAL_GATE") == "1"
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
// tier1CatastrophicPy is the always-on guard: catastrophic, unrecoverable
// commands are denied outright. Deliberately narrow (fail-OPEN on parse error)
// so ordinary build/test/git work is never blocked.
const tier1CatastrophicPy = `#!/usr/bin/env python3
import sys, json, re, os, subprocess
# Tier 1 — catastrophic, unrecoverable. Fail-open (never break a build).
DANGER = [
    r"rm\s+-rf?\s+(--no-preserve-root\s+)?/(\s|$|\*)",   # rm -rf /  or  /*
    r":\(\)\s*\{\s*:\|:&\s*\}\s*;:",                       # classic fork bomb
    r"\bmkfs(\.\w+)?\b",                                    # format a filesystem
    r"\bdd\b[^\n]*\bof=/dev/(sd|nvme|xvd|vd)",              # overwrite a raw disk
    r">\s*/dev/(sd|nvme|xvd|vd)",                           # redirect onto a raw disk
    r"docker\s+run\b[^\n]*-v\s*/:(/|\s)",                   # mount host root into a container
    r"\bchmod\s+-R\s+0*777\s+/(\s|$)",                      # world-writable root
]
def deny(reason):
    print(json.dumps({"hookSpecificOutput": {
        "hookEventName": "PreToolUse", "permissionDecision": "deny",
        "permissionDecisionReason": reason}}))
    sys.exit(0)
try:
    data = json.load(sys.stdin)
    cmd = (data.get("tool_input") or {}).get("command", "") or ""
except Exception:
    sys.exit(0)  # unparseable -> allow (fail-open, tier 1 only)
for pat in DANGER:
    if re.search(pat, cmd):
        deny("Blocked by Orynx guardrail: command matches a catastrophic pattern and was denied before execution.")
`

// tier2ApprovalPy is appended when the approval gate is on. Irreversible actions
// are denied unless a member has approved them (checked via the multica CLI).
// Fail-CLOSED for this tier: any check error denies. Only applies to issue-backed
// runs (MULTICA_ISSUE_ID present) — the approval attaches to an issue.
const tier2ApprovalPy = `
# Tier 2 — irreversible actions require human approval (P0.2). Fail-CLOSED.
IRREVERSIBLE = [
    r"\bgit\s+push\b", r"\bgh\s+pr\s+merge\b", r"\bgit\s+branch\s+-D\b",
    r"\bsudo\b",
    r"\b(drop|truncate)\s+(table|database|schema)\b",
    r"\balembic\s+(upgrade|downgrade)\b", r"\bmigrate\s+(up|down)\b",
    r"\bnpm\s+publish\b", r"\btwine\s+upload\b", r"\bcargo\s+publish\b", r"\bgem\s+push\b",
    r"\bterraform\s+apply\b", r"\bkubectl\s+(apply|delete)\b",
]
issue = os.environ.get("MULTICA_ISSUE_ID", "")
if issue:
    for pat in IRREVERSIBLE:
        if re.search(pat, cmd, re.I):
            decision = "error"
            try:
                out = subprocess.run(["multica","issue","approval","check","--issue",issue,"--command",cmd],
                                     capture_output=True, text=True, timeout=30)
                decision = (out.stdout or "").strip()
            except Exception:
                decision = "error"
            if decision == "approved":
                sys.exit(0)  # member-approved: allow this action once
            try:
                subprocess.run(["multica","issue","approval","request","--issue",issue,"--command",cmd,
                                "--task-id",os.environ.get("MULTICA_TASK_ID","")],
                               capture_output=True, text=True, timeout=30)
            except Exception:
                pass
            reason = "Blocked by Orynx approval gate: this irreversible action needs human approval. A request was posted for the operator; the action will run once approved."
            if decision == "rejected":
                reason = "Blocked by Orynx approval gate: the operator rejected this action."
            deny(reason)
`

// hookScript composes the PreToolUse hook: tier 1 always, tier 2 when the
// approval gate is enabled. Ends with sys.exit(0) (allow) so anything that
// matched no tier proceeds.
func hookScript(approvalGate bool) string {
	s := tier1CatastrophicPy
	if approvalGate {
		s += tier2ApprovalPy
	}
	return s + "\nsys.exit(0)\n"
}

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
	if !bashGuardEnabled() && !approvalGateEnabled() {
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
	if err := recordWriteFile(guardPath, []byte(hookScript(approvalGateEnabled())), 0o755, manifest); err != nil {
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
