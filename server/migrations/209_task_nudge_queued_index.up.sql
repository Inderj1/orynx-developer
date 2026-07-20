-- Backs the Stop hook's per-task queued-nudge lookup (consume path). Concurrent +
-- single-statement per house rules.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_task_nudge_queued
    ON task_nudge (workspace_id, task_id, status, created_at);
