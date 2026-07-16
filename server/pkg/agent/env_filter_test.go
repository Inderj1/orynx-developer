package agent

import (
	"strings"
	"testing"
)

// The agent env filter is a security boundary: untrusted agent content must not
// be handed the launching process's secrets, but the agent's own provider
// credentials must survive so it can authenticate.
func TestFilterInheritedEnv_stripsHostSecretsKeepsProviderKeys(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"HOME=/home/x",
		"LANG=en_US.UTF-8",
		// Host / daemon secrets — MUST be stripped.
		"JWT_SECRET=supersecret",
		"DATABASE_URL=postgres://u:p@h/db",
		"POSTGRES_PASSWORD=pw",
		"RESEND_API_KEY=re_x",
		"COMPOSIO_API_KEY=cmp",
		"MULTICA_LLM_API_KEY=llm",
		"GOOGLE_CLIENT_SECRET=gsecret",
		"SMTP_PASSWORD=smtppw",
		"MULTICA_SLACK_SECRET_KEY=slack",
		"MULTICA_DEV_VERIFICATION_CODE=888888",
		"SOME_CUSTOM_SECRET=x", // matched by pattern
		// Agent-provider credentials — MUST survive.
		"ANTHROPIC_API_KEY=sk-ant",
		"OPENAI_API_KEY=sk-oai",
		"XAI_API_KEY=xai",
		"GITHUB_TOKEN=ghp_x",
		"CLAUDE_CODE_GIT_BASH_PATH=/bin/bash",
	}
	got := map[string]bool{}
	for _, e := range filterInheritedEnv(in) {
		k, _, _ := strings.Cut(e, "=")
		got[k] = true
	}

	for _, k := range []string{
		"JWT_SECRET", "DATABASE_URL", "POSTGRES_PASSWORD", "RESEND_API_KEY",
		"COMPOSIO_API_KEY", "MULTICA_LLM_API_KEY", "GOOGLE_CLIENT_SECRET",
		"SMTP_PASSWORD", "MULTICA_SLACK_SECRET_KEY",
		"MULTICA_DEV_VERIFICATION_CODE", "SOME_CUSTOM_SECRET",
	} {
		if got[k] {
			t.Errorf("secret %q leaked into agent env (must be stripped)", k)
		}
	}
	for _, k := range []string{
		"PATH", "HOME", "LANG", "ANTHROPIC_API_KEY", "OPENAI_API_KEY",
		"XAI_API_KEY", "GITHUB_TOKEN", "CLAUDE_CODE_GIT_BASH_PATH",
	} {
		if !got[k] {
			t.Errorf("expected %q to be preserved for the agent, but it was stripped", k)
		}
	}
}
