# Design: Patch-from-payload for the agent-task-snapshot (kill the task-event refetch storm)

**Status:** ready to implement (needs a running multi-agent stack to load-validate)
**Owner:** unassigned
**Closes:** the remaining half of the "invalidation storm" roadmap item (the coalesce half — a 300ms task-prefix debounce — is already shipped in `packages/core/realtime/use-realtime-sync.ts`).

## Problem

Every `task:` lifecycle event (`queued/dispatch/running/completed/failed/cancelled`) triggers a debounced invalidation of ~7 workspace-wide query trees. The most expensive is `agentTaskSnapshotKeys.list(wsId)`, backed by
`SELECT DISTINCT ON (agent_id) … ORDER BY completed_at DESC` over `agent_task_queue` + `agent` — a full-workspace scan (now indexed via `(agent_id, completed_at DESC)`, migration 196, but still refetched over the network to **every connected client** on each debounced batch).

Under heavy agent load (N agents × M lifecycle events) this is N×M refetches. The coalesce (300ms debounce) reduces *frequency*; it does not remove the *refetch*. The win here is to **patch the snapshot cache directly from the event payload** and keep invalidation only as a reconciling fallback.

## Current state

- Coalesce window: `task` prefix debounced at **300ms** (`use-realtime-sync.ts`, `TASK_DEBOUNCE_MS`). Other prefixes 100ms.
- Task payloads are **lean** — `task_id, agent_id, issue_id, chat_session_id?, status` (`packages/core/types/events.ts`). Not enough to patch the snapshot row, which is why this needs a backend change first.

## Backend changes (`server/`)

1. **Enrich the task lifecycle payloads** so the frontend can reconstruct the snapshot entry for an agent without a refetch. Add to `TaskCompletedPayload` / `TaskFailedPayload` / `TaskCancelledPayload` / `TaskDispatchedPayload` / `TaskRunningPayload` / `TaskQueuedPayload` (in `server/internal/handler` / wherever they're emitted):
   - a `snapshot` sub-object matching the `agent_task_snapshot` row **1:1** — `agent_id`, `task_id`, `status`, `issue_id`, `issue_identifier`, `issue_title`, `started_at`, `completed_at`, `updated_at`, and the derived **agent presence** (`working`/`idle`). Populate from the task row already in hand at emit time.
   - Prefer one `snapshot` object over scattered fields → the frontend patch is a drop-in replace and it's trivially schema-validatable.
2. **Backward compatible / additive only** — installed desktop clients on older frontends must tolerate the new fields (they already ignore unknown JSON). Do not remove or rename existing payload fields.
3. Emit sites: find where `task:*` events are broadcast to the hub after a lifecycle transition; the snapshot fields come from the same query/row that transitioned the task.

## Frontend changes (`packages/core/`)

1. **Add an exported pure helper** (mirrors the `cache-coordinator.ts` pattern so it's unit-testable):
   `applyTaskEventToSnapshot(prev: AgentTaskSnapshotList, payload): AgentTaskSnapshotList`
   - Find the agent's current entry by `agent_id`.
   - **Respect `DISTINCT ON (agent_id) ORDER BY completed_at DESC`**: only overwrite if the incoming task is the agent's *latest* (compare `completed_at` / `updated_at`, not arrival order).
   - New agent (no entry) → append.
   - Terminal events (`completed/failed/cancelled`) → set presence `idle` **conservatively** (see edge cases); start events (`dispatch/running`) → `working`.
2. In the `task:` handler in `use-realtime-sync.ts`, `setQueryData(agentTaskSnapshotKeys.list(wsId), applyTaskEventToSnapshot(...))` **instead of** invalidating that one key.
3. **Keep a reconciling fallback**: retain a (longer, trailing) debounced `invalidateQueries` for the snapshot so correctness never depends solely on the patch — if the payload is missing fields or the cache is absent, the invalidation still reconciles. Net effect: the common case avoids the refetch; the rare case self-heals.
4. **Leave the cheap aggregates as-is** — `agentRunCounts` / `agentActivity` are 30-day rollups; patching them is fussy and low-value. Keep those debounced-invalidated. The snapshot is the whole win.
5. **Validate the enriched payload** with a zod schema at the WS boundary (`parseWithFallback`) — this dovetails with the separate "WS payload schemas" backlog item; do the `task:*` schemas here.

## Edge cases

- **Out-of-order delivery** — a `running` arriving after `completed` for the same task: decide the winning state by timestamp, never by arrival order.
- **Queued follow-on task** — an agent that finishes one task but has another queued should stay `working`; a naive "terminal → idle" is wrong. The reconciling invalidation covers this; prefer patching presence conservatively (don't flip to idle on terminal if payload signals a queued successor) and let reconciliation settle.
- **Multi-tab / multi-client** — every client patches its own cache from the same broadcast; the helper must be idempotent.
- **Workspace switch / stale wsId** — already guarded by `getCurrentWsId()`.

## Testing plan

- **Unit** (no stack): tests for `applyTaskEventToSnapshot` — newer task replaces, older task ignored, new agent appended, terminal→idle, start→working, DISTINCT-ON winner selection, idempotency. Mirror `cache-coordinator.test.ts`.
- **Integration / load** (needs running stack — the EC2 works): trigger N agents completing near-simultaneously; assert the snapshot cache updates **without a network refetch** (spy the query fn / network panel), and that presence matches a forced refetch. Compare refetch counts before/after.
- **Regression**: the existing 33 `use-realtime-sync.test.ts` cases must stay green.

## Rollout / risk

- Additive payload fields → safe for installed clients (API compatibility rules).
- Frontend patch sits **behind** the reconciling invalidation → a wrong patch is corrected within the debounce window; low blast radius.
- Ship order: (1) backend payload enrichment (no behavior change), (2) frontend zod schema + pure helper + tests, (3) swap invalidate→patch with the fallback, (4) load-validate on the EC2.

## Effort

- Backend payload enrichment: **S–M** — add a `snapshot` sub-object + populate at emit sites + keep additive.
- Frontend pure helper + patch + zod schema + tests: **M**.
- Load validation on the EC2: **S**.
