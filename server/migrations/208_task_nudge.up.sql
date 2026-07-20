-- Mid-run steering (Phase 2). A "nudge" is a human-authored steering message
-- queued for a RUNNING agent task. The agent's Stop hook consumes queued nudges
-- at a turn boundary and continues with them, so an operator can redirect a run
-- without cancelling it (Orynx runs are otherwise terminal/one-shot).
--
-- No foreign keys per house rules; rows are cleaned up in application code on
-- workspace delete (see DeleteNudgesByWorkspace + the delete-coverage allowlist).
CREATE TABLE task_nudge (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    -- The running task being steered.
    task_id UUID NOT NULL,
    content TEXT NOT NULL,
    -- The member who sent the nudge (steering is a human action).
    author_member UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'delivered')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ
);
