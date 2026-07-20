-- Molecule spine: issue dependency DAG + is_blocked recompute + ready-work read.
-- See migrations 203-207. Dependencies are workspace-scoped; there are no FKs, so
-- edges touching a deleted issue are cleaned up here from application code.

-- name: CreateDependency :one
-- Idempotent add of an edge (issue_id depends on depends_on_id). Re-adding an
-- existing edge just updates its type.
INSERT INTO issue_dependency (workspace_id, issue_id, depends_on_id, dep_type)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id, issue_id, depends_on_id)
DO UPDATE SET dep_type = EXCLUDED.dep_type
RETURNING *;

-- name: DeleteDependency :exec
DELETE FROM issue_dependency
WHERE workspace_id = $1 AND issue_id = $2 AND depends_on_id = $3;

-- name: DeleteDependenciesForIssue :exec
-- Cleanup on issue delete: drop every edge touching this issue (either endpoint).
-- Call inside the same transaction that deletes the issue (no FK cascade).
DELETE FROM issue_dependency
WHERE workspace_id = $1 AND (issue_id = $2 OR depends_on_id = $2);

-- name: ListDependenciesForIssue :many
-- What this issue depends on (its blockers).
SELECT * FROM issue_dependency
WHERE workspace_id = $1 AND issue_id = $2
ORDER BY created_at;

-- name: ListDependentsOf :many
-- Which issues depend on this one — recompute targets when it closes.
SELECT * FROM issue_dependency
WHERE workspace_id = $1 AND depends_on_id = $2
ORDER BY created_at;

-- name: RecomputeWorkspaceBlocked :exec
-- Recompute issue.is_blocked for a whole workspace. An active issue is blocked if
-- it has a 'blocks' edge on a non-terminal blocker, and blocked-ness propagates
-- DOWN the epic tree (a child of a blocked issue is blocked). Only rows whose
-- blocked state actually changes are written, to minimize churn/realtime events.
WITH RECURSIVE directly_blocked AS (
    SELECT DISTINCT d.issue_id AS id
    FROM issue_dependency d
    JOIN issue dep
      ON dep.id = d.issue_id AND dep.workspace_id = d.workspace_id
    JOIN issue blocker
      ON blocker.id = d.depends_on_id AND blocker.workspace_id = d.workspace_id
    WHERE d.workspace_id = $1
      AND d.dep_type = 'blocks'
      AND dep.status NOT IN ('done', 'cancelled')
      AND blocker.status NOT IN ('done', 'cancelled')
),
blocked_tree AS (
    SELECT id FROM directly_blocked
    UNION
    SELECT child.id
    FROM issue child
    JOIN blocked_tree bt ON child.parent_issue_id = bt.id
    WHERE child.workspace_id = $1
      AND child.status NOT IN ('done', 'cancelled')
)
UPDATE issue
SET is_blocked = (issue.id IN (SELECT id FROM blocked_tree)),
    updated_at = now()
WHERE issue.workspace_id = $1
  AND is_blocked <> (issue.id IN (SELECT id FROM blocked_tree));

-- name: GetReadyIssues :many
-- Dispatchable work for the molecule spine: active, unblocked issues in a
-- workspace, highest priority then board order. (A live-lease filter is layered
-- on by the daemon in the resume increment.)
SELECT * FROM issue
WHERE workspace_id = $1
  AND is_blocked = false
  AND status IN ('backlog', 'todo', 'in_progress')
ORDER BY
    CASE priority
        WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2
        WHEN 'low' THEN 3 ELSE 4
    END,
    position,
    created_at
LIMIT sqlc.arg('lim');
