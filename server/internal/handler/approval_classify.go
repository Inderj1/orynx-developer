package handler

import "regexp"

// Approval-gate action classification — the security floor. Every gated command
// is sorted into a tier: 'operational' (learnable — a member can approve once and
// teach a policy) or 'security' (NEVER learnable — always asks). Getting this
// split right is the uncompromisable part: a security action must never be
// classifiable as operational, or it could be auto-approved.
//
// The hook only calls the gate for commands matching its broad irreversible
// pre-filter, so this classifier mostly sees operational/security candidates;
// anything it doesn't recognize is treated as security (fail-closed).

const (
	tierOperational = "operational"
	tierSecurity    = "security"
)

// securityPatterns are NEVER learnable — they always require a fresh human
// decision (the user's confirmed red line: prod deploy, DB drop/destructive
// migration, secret/credential change, spend, access grants, protected-branch
// force-push, box security config). Order matters: security is checked first so
// a command that matches both tiers is classified security.
var securityPatterns = []struct {
	re    *regexp.Regexp
	class string
}{
	{regexp.MustCompile(`(?i)\bsudo\b`), "box_security"},
	{regexp.MustCompile(`(?i)\b(drop|truncate)\s+(table|database|schema)\b`), "db_destructive"},
	{regexp.MustCompile(`(?i)\balembic\s+downgrade\b`), "db_destructive"},
	{regexp.MustCompile(`(?i)\bgit\s+push\b[^\n]*(--force\b|--force-with-lease\b|\s-f\b)`), "force_push"},
	{regexp.MustCompile(`(?i)\b(npm\s+publish|twine\s+upload|cargo\s+publish|gem\s+push)\b`), "package_publish"},
	{regexp.MustCompile(`(?i)\bterraform\s+apply\b`), "infra_apply"},
	{regexp.MustCompile(`(?i)\bkubectl\s+(apply|delete)\b`), "k8s_apply"},
	{regexp.MustCompile(`(?i)\b(aws\s+configure|gh\s+auth|git\s+credential|ssh-keygen)\b`), "credential_change"},
	{regexp.MustCompile(`(?i)\brm\s+-rf?\b[^\n]*(/etc/|/home/|~|\$HOME|\.\.)`), "destructive_delete"},
	{regexp.MustCompile(`(?i)\b(stripe|payment|transfer|payout|withdraw)\b`), "spend"},
}

// operationalPatterns are learnable — approve once, then a policy auto-approves
// the class. Everyday-but-consequential actions.
var operationalPatterns = []struct {
	re    *regexp.Regexp
	class string
}{
	{regexp.MustCompile(`(?i)\bgh\s+pr\s+merge\b`), "gh_pr_merge"},
	{regexp.MustCompile(`(?i)\bgit\s+push\b`), "git_push"}, // plain push (force is caught above as security)
	{regexp.MustCompile(`(?i)\bgit\s+branch\s+-D\b`), "branch_delete"},
	{regexp.MustCompile(`(?i)\b(alembic\s+upgrade|migrate\s+up)\b`), "db_migrate_up"},
}

// classifyCommand returns the tier and action class of a gated command. Security
// is matched first (a command matching both tiers is security). An unrecognized
// command that reached the gate is treated as security (fail-closed) — the caller
// must ask a human rather than guess it's safe.
func classifyCommand(cmd string) (tier, actionClass string) {
	for _, p := range securityPatterns {
		if p.re.MatchString(cmd) {
			return tierSecurity, p.class
		}
	}
	for _, p := range operationalPatterns {
		if p.re.MatchString(cmd) {
			return tierOperational, p.class
		}
	}
	// Reached the gate but matched no known class → treat as security: never
	// auto-approvable, always asks. Safer than assuming it's benign.
	return tierSecurity, "unclassified"
}
