-- Issue dependency graph — the DAG behind "ready work" and the molecule spine.
--
-- 001_init defined an issue_dependency table but nothing ever used it (no queries,
-- no handlers, no frontend), and it predates the house rules (foreign keys, no
-- workspace_id). Since the feature was never live, we replace the dead shape with
-- a workspace-scoped, FK-free one rather than preserve both.
--
-- A row means: issue_id is blocked by depends_on_id when dep_type = 'blocks'
-- (issue_id waits for depends_on_id to reach a terminal status). Orynx already
-- models epics via parent_issue_id and ordering via stage; dependencies add the
-- cross-cutting blocks/waits edges that let the daemon compute what is dispatchable.
--
-- No foreign keys per house rules; edges touching a deleted issue are cleaned up
-- in application code within the deleting transaction.
DROP TABLE IF EXISTS issue_dependency;

CREATE TABLE issue_dependency (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    -- The dependent issue (the one that becomes blocked).
    issue_id UUID NOT NULL,
    -- The issue it depends on (the blocker).
    depends_on_id UUID NOT NULL,
    dep_type TEXT NOT NULL DEFAULT 'blocks'
        CHECK (dep_type IN ('blocks', 'related')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
