-- Backs the daemon's ready-work read: dispatchable issues in a workspace
-- (not blocked, active status). Partial-free composite so the planner can filter
-- on workspace_id + is_blocked and range/equality on status.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_ready
    ON issue (workspace_id, is_blocked, status);
