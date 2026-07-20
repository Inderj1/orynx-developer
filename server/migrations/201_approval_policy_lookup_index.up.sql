-- Backs the hook's policy check (match on workspace + action_class, filtered by
-- agent/resource/revoked in app code). Concurrent, single-statement per house rules.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_approval_policy_lookup
    ON approval_policy (workspace_id, action_class);
