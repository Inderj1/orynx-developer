-- Mid-run steering (Phase 2): queued nudges for a running task. See migrations 208/209.

-- name: CreateNudge :one
-- Enqueue a human-authored steering message for a running task.
INSERT INTO task_nudge (workspace_id, task_id, content, author_member)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ConsumeQueuedNudges :many
-- Atomically mark every queued nudge for a task delivered and return their
-- content, oldest first. Called by the agent's Stop hook at a turn boundary, so
-- each nudge is delivered exactly once even if the hook fires concurrently.
WITH consumed AS (
    UPDATE task_nudge
    SET status = 'delivered', delivered_at = now()
    WHERE workspace_id = $1 AND task_id = $2 AND status = 'queued'
    RETURNING content, created_at
)
SELECT content FROM consumed ORDER BY created_at;

-- name: ListNudgesForTask :many
-- Nudge history for a task (UI / audit), most recent first.
SELECT * FROM task_nudge
WHERE workspace_id = $1 AND task_id = $2
ORDER BY created_at DESC;

-- name: DeleteNudgesByWorkspace :exec
-- Workspace-delete cleanup (no FK cascade). Called in the DeleteWorkspace tx.
DELETE FROM task_nudge WHERE workspace_id = $1;
