-- Approval-gate learning layer queries. See migrations 199/200.

-- name: CreatePolicy :one
-- Learn a rule from a one-time approval (only operational-tier actions).
INSERT INTO approval_policy (
    workspace_id, agent_id, action_class, resource, source_approval_id, created_by_member
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: FindMatchingPolicies :many
-- The hook's policy check: candidate rules for (workspace, action_class) that are
-- not revoked. Agent/resource scoping is applied in app code (a NULL agent_id or
-- resource on the policy means "any"), so a single query serves the match.
SELECT * FROM approval_policy
WHERE workspace_id = $1
  AND action_class = $2
  AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: GetPolicy :one
SELECT * FROM approval_policy
WHERE id = $1 AND workspace_id = $2;

-- name: RevokePolicy :one
-- Correctability: stop future auto-approvals from this rule. Row kept for audit.
UPDATE approval_policy
SET revoked_at = now(), revoked_by = sqlc.arg('revoked_by')
WHERE id = sqlc.arg('id') AND workspace_id = sqlc.arg('workspace_id') AND revoked_at IS NULL
RETURNING *;

-- name: ListPolicies :many
-- The "rules view": all learned rules for the workspace, active first.
SELECT * FROM approval_policy
WHERE workspace_id = $1
ORDER BY (revoked_at IS NULL) DESC, created_at DESC;

-- name: LogDecision :one
-- Append to the "why" feed: one row per gated decision with its reasoning.
INSERT INTO approval_decision_log (
    workspace_id, issue_id, task_id, agent_id, action_class, command, tier, decision, policy_id, reason
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: ListDecisions :many
-- The decision-log feed (most recent first), optionally scoped to an issue.
SELECT * FROM approval_decision_log
WHERE workspace_id = $1
  AND (sqlc.narg('issue_id')::uuid IS NULL OR issue_id = sqlc.narg('issue_id'))
ORDER BY created_at DESC
LIMIT sqlc.arg('lim');

-- name: DeleteApprovalPoliciesByWorkspace :exec
-- Workspace-delete cleanup (no FK cascade). Called in the DeleteWorkspace tx.
DELETE FROM approval_policy WHERE workspace_id = $1;

-- name: DeleteApprovalDecisionLogByWorkspace :exec
-- Workspace-delete cleanup (no FK cascade). Called in the DeleteWorkspace tx.
DELETE FROM approval_decision_log WHERE workspace_id = $1;
