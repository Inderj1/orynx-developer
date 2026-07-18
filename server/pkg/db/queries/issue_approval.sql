-- Approval gate (P0.2) queries. See migration 197.

-- name: CreateApproval :one
-- Records a pending irreversible action. Idempotency is handled in the app:
-- the handler reuses an existing pending row for the same (issue_id, action_key)
-- rather than creating duplicates when an agent retries the same command.
INSERT INTO issue_approval (
    workspace_id, issue_id, task_id, action_key, command, requested_by_agent
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetLatestApprovalForAction :one
-- The hook's approval check: is there a decision for this exact action on this
-- issue? Newest wins (a fresh request supersedes an old consumed/rejected one).
SELECT * FROM issue_approval
WHERE issue_id = $1 AND action_key = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: GetPendingApprovalForAction :one
-- Whether a still-pending request already exists for this action (idempotent
-- request: don't create a second inbox item on an agent retry).
SELECT * FROM issue_approval
WHERE issue_id = $1 AND action_key = $2 AND status = 'pending'
ORDER BY created_at DESC
LIMIT 1;

-- name: GetApproval :one
SELECT * FROM issue_approval
WHERE id = $1 AND workspace_id = $2;

-- name: DecideApproval :one
-- Member decision (approve / reject). Only transitions a pending row; a row
-- already decided/consumed is left as-is (returns nothing). The handler enforces
-- member-only actor before calling this.
UPDATE issue_approval
SET status = sqlc.arg('status'),
    decided_by_member = sqlc.arg('decided_by_member'),
    decided_at = now()
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id')
  AND status = 'pending'
RETURNING *;

-- name: ConsumeApproval :one
-- Single-use: the hook consumes an 'approved' row the first time it allows the
-- action, so one approval authorizes exactly one execution. Only an 'approved'
-- row transitions; a concurrent second run finds it already 'consumed'.
UPDATE issue_approval
SET status = 'consumed', consumed_at = now()
WHERE id = sqlc.arg('id')
  AND status = 'approved'
RETURNING *;

-- name: ListPendingApprovalsForWorkspace :many
SELECT * FROM issue_approval
WHERE workspace_id = $1 AND status = 'pending'
ORDER BY created_at DESC;
