-- Backs the hook's approval check (GetLatestApprovalForAction: WHERE issue_id =
-- $1 AND action_key = $2 ORDER BY created_at DESC) — hit on every irreversible
-- command an agent attempts. Keep as the migration's only statement:
-- PostgreSQL rejects CREATE INDEX CONCURRENTLY inside a transaction.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_approval_issue_action
    ON issue_approval (issue_id, action_key, created_at DESC);
