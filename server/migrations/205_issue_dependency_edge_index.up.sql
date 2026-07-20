-- Dedupes edges (one row per issue_id -> depends_on_id) and backs
-- ListDependenciesForIssue (WHERE workspace_id = $1 AND issue_id = $2 via prefix).
-- Concurrent + single-statement per house rules.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_dependency_edge
    ON issue_dependency (workspace_id, issue_id, depends_on_id);
