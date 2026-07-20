-- Denormalized "is this issue currently blocked by an unmet dependency?".
-- Recomputed (not queried live) so the daemon's ready-work read is a single
-- indexed scan instead of a recursive walk on every dispatch. Maintained by the
-- application after any dependency or blocker-status change — see the
-- RecomputeWorkspaceBlocked query. A constant default keeps ADD COLUMN fast
-- (no table rewrite on PostgreSQL 11+).
ALTER TABLE issue ADD COLUMN is_blocked BOOLEAN NOT NULL DEFAULT false;
