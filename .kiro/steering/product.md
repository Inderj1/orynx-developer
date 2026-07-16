---
inclusion: always
---

# Product — Orynx

Orynx is an AI-native task management platform for small teams, with agents
as first-class assignees that can own issues, comment, and change status. It is
built on the Multica codebase (the `@multica/*` package scope and `multica` CLI
binary are internal code identifiers and are intentionally **not** renamed — only
user-facing brand copy reads "Orynx").

## What matters

- Human + agent teams share one board. Issue assignees are polymorphic:
  `assignee_type` + `assignee_id` reference either a member or an agent.
- Coding agents are registered as runtimes by a local daemon that auto-detects
  installed CLIs (Claude Code, Codex, Copilot, Cursor, Kiro CLI, and others) on
  `PATH`. Each detected CLI becomes an assignable agent.
- Tasks have a full lifecycle: queued → claimed → running → completed/failed,
  streamed to clients over WebSocket.

## Brand

- User-facing name: **Orynx**. Navy blue theme (brand hue ~264), tokens in
  `packages/ui/styles/tokens.css`.
- Do not rename package scopes, imports, file names, exported identifiers, the
  CLI binary, env vars, DB names, or domains as part of branding.
