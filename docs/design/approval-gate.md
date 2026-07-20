# Approval gate — human-in-the-loop for irreversible agent actions

Status: proposed · Owner: platform/security · Tracking: punch-list P0.2

## Problem

Agents run Claude Code with `--permission-mode bypassPermissions` and
`--disallowedTools AskUserQuestion` (server/pkg/agent/claude.go:574,581), so they
**cannot pause and ask** before an irreversible action. The Orchestrator prompt
that says "STOP and ask before irreversible actions" is therefore unenforceable —
git push, `sudo`, DB `DROP`/migration, `rm -rf` outside the workdir, deploy, and
package publish all run unprompted. This is the core walk-away-safety gap.

## Constraint from the platform's shape

- **A run is discrete and terminal-only.** agent_task_queue statuses are
  queued/dispatched/running/completed/failed/cancelled — there is **no
  suspend/resume**. So "pause" = end this run, re-dispatch a fresh one on approval.
- **`deny` is the only synchronous gate.** A PreToolUse hook's
  `permissionDecision:"deny"` blocks the tool *before* it runs, regardless of
  permission mode. Task cancellation only interrupts after a ~5s poll — too late
  to stop the action. So the gate MUST be the hook's `deny`, never cancellation.
- **The approval must not be agent-forgeable.** `SetIssueMetadataKey`
  (server/internal/handler/issue_metadata.go:139-199) authorizes with
  `requireUserID` only and does NOT reject `actorType == "agent"` — an agent's
  own task token can write issue metadata. So the approval record CANNOT be a
  plain metadata key (the agent would self-approve). It must live in a store the
  agent cannot write.

## Design — deny → notify → approve → allow

### The store (agent-unwritable — the crux)
New table `issue_approval` (no FK per house rules; `issue_id` resolved in app code):
```
issue_approval(
  id UUID PK, workspace_id UUID, issue_id UUID, task_id UUID,
  action_key TEXT,        -- stable id of the pending action (see below)
  command TEXT,           -- the exact command, for the human to review
  status TEXT,            -- 'pending' | 'approved' | 'rejected' | 'consumed'
  requested_by_agent UUID,
  decided_by_member UUID, -- NULL until a MEMBER decides
  created_at, decided_at, consumed_at )
```
The **approve/reject writer is member-only** — the handler rejects
`actorType == "agent"` (the info is in `resolveActor`). The request writer and the
status reader are agent-safe. This is what stops self-approval.

### action_key
`sha256(issueID + ":" + normalizedCommand)` where normalizedCommand collapses
whitespace. Single-use: an approval is `consumed` the first time the hook allows
it, so one approval authorizes exactly one execution of that command. Granularity
tradeoff (exact-hash = brittle across a fresh run; loose = over-authorizes) is the
core design risk — start exact-command + single-use; iterate to a per-issue
"approved action class" only if re-runs prove too brittle.

### Flow
1. **Classify (extend server/internal/daemon/execenv/managed_settings.go).** Add a
   tier-2 "irreversible" matcher alongside the existing tier-1 catastrophic guard.
   Tier 2 is **fail-CLOSED** (deny on any classify/lookup error) — the opposite of
   tier-1's fail-open. Gated by a new env `MULTICA_AGENT_APPROVAL_GATE=1`.
2. **Check approval first.** Hook calls `multica issue approval status
   --issue $ISSUE --action-key <key>` (read-only, agent-safe). If `approved` →
   emit `permissionDecision:"allow"` and the CLI marks it `consumed` (single-use).
3. **If not approved → `deny`** (the real gate), then the hook:
   - `multica issue approval request --issue $ISSUE --task $TASK --action-key <key>
     --command "<cmd>"` → creates the `issue_approval` row (pending) AND an inbox
     item `type:"approval_required" severity:"action_required"
     details:{command,action_key,task_id}`.
   - `multica issue status $ISSUE blocked` (visible "awaiting approval").
   - ends its turn (nothing else to do — the action is denied).
4. **Human approves** in the UI / CLI (`multica issue approval approve <id>`):
   member-only endpoint records `approved` + `decided_by_member`, archives the
   inbox item, and re-triggers a run (flip `blocked→todo`, which enqueues via
   WillEnqueueRun, or post an `@agent` comment).
5. **Resume + allow:** the fresh run reaches the same command; step 2 now returns
   `approved` → hook allows → the CLI consumes the token.

### Irreversible tier-2 command set (start narrow, expand)
`git push` / `git push --force` / `gh pr merge` / branch delete; any `sudo`; any
`docker`/`psql` `DROP`/`DELETE`/`TRUNCATE`/DDL or a migration command; `rm -rf`
targeting outside the task workdir; package publish (`npm publish`, `pip upload`,
`cargo publish`); `terraform apply`; outbound `curl -X POST/PUT` to a
non-allowlisted host. Reversible everyday work (build, test, read, local git
commit, `docker compose up` in the workdir) is untouched.

## Build order (each independently shippable)
1. `issue_approval` table + queries (migration, no FK, CONCURRENTLY index).
2. Endpoints: `POST /approvals` (request, agent-safe), `GET /approvals/status`
   (agent-safe), `POST /approvals/{id}/decide` (member-only). CLI verbs
   `issue approval request|status|approve|reject`.
3. Hook tier-2 classifier + approval check (managed_settings.go), fail-closed,
   env-gated.
4. Inbox UI: render `approval_required` with Approve/Reject buttons (interim: the
   operator approves via the CLI).
5. Resume wiring (approve → re-trigger).

## Rollout
Default OFF (`MULTICA_AGENT_APPROVAL_GATE` unset). Turn on per-box after the
endpoints + CLI + hook are deployed and a canary proves: an agent `git push` is
denied + queued; the operator approves; the re-run pushes; and the agent cannot
self-approve (a `POST /approvals/{id}/decide` with an agent token is rejected).
