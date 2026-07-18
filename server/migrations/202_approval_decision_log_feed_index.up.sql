-- Backs the decision-log "why" feed (most-recent-first per workspace).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_approval_decision_log_feed
    ON approval_decision_log (workspace_id, created_at DESC);
