-- Approval-gate decision log (autonomy/explainability). One row per gated
-- decision: what the action was, what the gate decided, and WHY (which policy
-- applied, or that it was denied pending approval). This is the running "why did
-- you do that" feed the operator can inspect to understand — and correct — the
-- brain-replica's behavior. No foreign keys per house rules.
CREATE TABLE approval_decision_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID,
    task_id UUID,
    agent_id UUID,
    action_class TEXT NOT NULL,
    command TEXT NOT NULL,
    tier TEXT NOT NULL,            -- 'operational' | 'security'
    decision TEXT NOT NULL,        -- 'allowed_by_policy' | 'approved_once' | 'denied_pending' | 'denied_rejected'
    policy_id UUID,                -- the learned rule that applied (allowed_by_policy)
    reason TEXT NOT NULL,          -- human-readable "why"
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
