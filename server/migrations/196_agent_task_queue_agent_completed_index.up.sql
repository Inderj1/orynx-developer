-- Backs the per-agent "latest run" snapshot (ListWorkspaceAgentTaskSnapshot:
-- DISTINCT ON (agent_id) ... ORDER BY completed_at DESC), which is refetched
-- on every task lifecycle event. Without this, that hot path scans/sorts full
-- task history per agent. Keep as the migration's only statement: PostgreSQL
-- rejects CREATE INDEX CONCURRENTLY inside a transaction or multi-command string.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_task_queue_agent_completed
    ON agent_task_queue (agent_id, completed_at DESC);
