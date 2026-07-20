-- Approval gate (P0.2): human-in-the-loop for irreversible agent actions.
--
-- One row per pending/decided irreversible action an agent tried to run (git
-- push, sudo, DB DROP/migration, rm -rf outside the workdir, deploy, publish).
-- The PreToolUse hook denies the action, creates a pending row here + an
-- inbox item, and ends the run; a MEMBER approves (issue_approval.status ->
-- 'approved', decided_by_member set); the re-dispatched run's hook sees the
-- approval and allows the action once (status -> 'consumed', single-use).
--
-- SECURITY: the approve/reject WRITE must be member-only (the handler rejects
-- actorType == 'agent'), because an agent's own task token can otherwise write
-- issue metadata (SetIssueMetadataKey has no agent-actor gate) and would
-- self-approve. This table is the agent-unwritable approval store.
--
-- No foreign keys / cascades per house rules — workspace_id / issue_id / task_id
-- relationships and cleanup are resolved in application code.
CREATE TABLE issue_approval (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    task_id UUID,
    -- Stable id of the pending action: sha256(issue_id + ':' + normalized command).
    -- Single-use: one approval authorizes exactly one execution of that command.
    action_key TEXT NOT NULL,
    -- The exact command, surfaced to the human for review.
    command TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected', 'consumed')),
    requested_by_agent UUID,
    -- NULL until a MEMBER (never an agent) decides.
    decided_by_member UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ,
    consumed_at TIMESTAMPTZ
);
