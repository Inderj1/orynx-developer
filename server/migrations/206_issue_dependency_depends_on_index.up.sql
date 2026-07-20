-- Reverse lookup: "which issues depend on X?" — hit when X reaches a terminal
-- status so the app can recompute the issues X may have unblocked.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_dependency_depends_on
    ON issue_dependency (workspace_id, depends_on_id);
