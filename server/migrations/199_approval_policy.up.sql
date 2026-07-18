-- Approval-gate learning layer (autonomy). A learned rule that auto-approves a
-- CLASS of operational action so the human is asked once, not every time. Each
-- rule carries its provenance (source_approval_id + who + when) so the operator
-- can always ask "why" and revoke it later — the record IS the explanation.
--
-- SECURITY: only 'operational'-tier actions ever become policies. Security-floor
-- actions (prod deploy, DB drop, secrets, spend, access, protected-branch
-- force-push, box security) are never learnable and never get a row here.
--
-- No foreign keys per house rules; relationships resolved in app code.
CREATE TABLE approval_policy (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    -- NULL agent_id = applies to any agent; else scoped to one agent.
    agent_id UUID,
    -- The action class this rule allows, e.g. 'git_push', 'gh_pr_merge',
    -- 'package_install'. Never a security-floor class.
    action_class TEXT NOT NULL,
    -- Optional resource scope (e.g. a repo). NULL = any resource in the class.
    resource TEXT,
    -- Provenance: the one-time approval this rule was learned from.
    source_approval_id UUID,
    created_by_member UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Correctability: revoking stops future auto-approvals; the row is kept for
    -- the audit trail (who learned it, who revoked it, when).
    revoked_at TIMESTAMPTZ,
    revoked_by UUID
);
