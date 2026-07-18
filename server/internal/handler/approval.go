package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Approval gate (P0.2). Irreversible agent actions are denied by the PreToolUse
// hook, which posts a pending approval here; a MEMBER approves; the re-dispatched
// run's hook checks + consumes the approval and the action proceeds once.
//
// SECURITY: the decide endpoint rejects an agent actor — an agent's own task
// token can write issue metadata (SetIssueMetadataKey has no agent gate), so if
// approvals were forgeable an agent would self-approve. This store is the
// agent-unwritable record; only a member can approve.

// ComputeActionKey is the stable id of a pending action: sha256(issueID + ":" +
// normalized command). Whitespace is collapsed so trivial formatting differences
// across a fresh run still match. Single-use is enforced by consuming on allow.
func ComputeActionKey(issueID, command string) string {
	norm := strings.Join(strings.Fields(command), " ")
	sum := sha256.Sum256([]byte(issueID + ":" + norm))
	return hex.EncodeToString(sum[:])
}

type approvalResponse struct {
	ID        string  `json:"id"`
	IssueID   string  `json:"issue_id"`
	TaskID    *string `json:"task_id"`
	ActionKey string  `json:"action_key"`
	Command   string  `json:"command"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
}

func approvalToResponse(a db.IssueApproval) approvalResponse {
	return approvalResponse{
		ID:        uuidToString(a.ID),
		IssueID:   uuidToString(a.IssueID),
		TaskID:    uuidToPtr(a.TaskID),
		ActionKey: a.ActionKey,
		Command:   a.Command,
		Status:    a.Status,
		CreatedAt: timestampToString(a.CreatedAt),
	}
}

// RequestApproval (POST /api/issues/{id}/approvals) — agent-safe. The hook calls
// this when an irreversible command is denied: it records a pending approval and
// raises an action-required inbox item for the accountable human. Idempotent:
// an agent retrying the same command reuses the existing pending row and does not
// spawn a second inbox item.
func (h *Handler) RequestApproval(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "id")
	issue, ok := h.loadIssueForUser(w, r, issueID)
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r) // the accountable human (from the task token)
	if !ok {
		return
	}
	var req struct {
		Command string `json:"command"`
		TaskID  string `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Command) == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	workspaceID := uuidToString(issue.WorkspaceID)
	actionKey := ComputeActionKey(uuidToString(issue.ID), req.Command)

	// Idempotent: reuse an existing pending request for this exact action.
	if existing, err := h.Queries.GetPendingApprovalForAction(r.Context(), db.GetPendingApprovalForActionParams{
		IssueID: issue.ID, ActionKey: actionKey,
	}); err == nil {
		writeJSON(w, http.StatusOK, approvalToResponse(existing))
		return
	}

	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	var agentID pgtype.UUID
	if actorType == "agent" {
		agentID = parseUUID(actorID)
	}
	created, err := h.Queries.CreateApproval(r.Context(), db.CreateApprovalParams{
		WorkspaceID:      issue.WorkspaceID,
		IssueID:          issue.ID,
		TaskID:           looseTaskUUID(req.TaskID),
		ActionKey:        actionKey,
		Command:          req.Command,
		RequestedByAgent: agentID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record approval request")
		return
	}

	// Raise an action-required inbox item for the accountable human so they can
	// review + decide. Best-effort: a failed inbox write must not lose the row.
	details, _ := json.Marshal(map[string]any{
		"command": req.Command, "action_key": actionKey, "approval_id": uuidToString(created.ID),
		"task_id": req.TaskID,
	})
	_, _ = h.Queries.CreateInboxItem(r.Context(), db.CreateInboxItemParams{
		WorkspaceID:   issue.WorkspaceID,
		RecipientType: "member",
		RecipientID:   parseUUID(userID),
		Type:          "approval_required",
		Severity:      "action_required",
		IssueID:       issue.ID,
		Title:         "Approval needed: " + truncate(req.Command, 80),
		Body:          pgtype.Text{String: "An agent wants to run an irreversible action. Review and approve or reject.", Valid: true},
		ActorType:     pgtype.Text{String: actorType, Valid: true},
		ActorID:       parseUUID(actorID),
		Details:       details,
	})
	h.publish(protocol.EventInboxNew, workspaceID, actorType, actorID, map[string]any{"issue_id": uuidToString(issue.ID)})
	writeJSON(w, http.StatusCreated, approvalToResponse(created))
}

// logApprovalDecision appends a row to the "why" feed. Best-effort — a failed
// log write must never change the gate decision.
func (h *Handler) logApprovalDecision(ctx context.Context, issue db.Issue, agentID pgtype.UUID, actionClass, command, tier, decision, reason string, policyID pgtype.UUID) {
	_, _ = h.Queries.LogDecision(ctx, db.LogDecisionParams{
		WorkspaceID: issue.WorkspaceID,
		IssueID:     issue.ID,
		AgentID:     agentID,
		ActionClass: actionClass,
		Command:     command,
		Tier:        tier,
		Decision:    decision,
		PolicyID:    policyID,
		Reason:      reason,
	})
}

// CheckApproval (POST /api/issues/{id}/approvals/check) — agent-safe. The gate's
// brain. Classifies the command, and:
//   - operational tier: auto-approves if a LEARNED POLICY matches (this is the
//     "asked once, never again"); else falls back to a one-time approval.
//   - security tier: NEVER consults policies — only an explicit one-time approval
//     lets it through. This is the uncompromisable floor.
// Every decision is logged with its "why". Response: {"decision", "tier",
// "action_class"} where decision ∈ approved|pending|rejected|none.
func (h *Handler) CheckApproval(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "id")
	issue, ok := h.loadIssueForUser(w, r, issueID)
	if !ok {
		return
	}
	userID, _ := requireUserID(w, r)
	var req struct {
		Command   string `json:"command"`
		ActionKey string `json:"action_key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	command := strings.TrimSpace(req.Command)
	if command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	actionKey := ComputeActionKey(uuidToString(issue.ID), command)
	tier, actionClass := classifyCommand(command)
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	var agentID pgtype.UUID
	if actorType == "agent" {
		agentID = parseUUID(actorID)
	}

	// Operational tier only: a matching learned policy auto-approves — the
	// "asked once, remembered forever". Security tier skips this entirely.
	if tier == tierOperational {
		if policy, matched := h.matchPolicy(r.Context(), issue.WorkspaceID, agentID, actionClass); matched {
			reason := "Auto-approved by a learned rule (policy " + uuidToString(policy.ID)[:8] + ") — you approved this class of action before."
			h.logApprovalDecision(r.Context(), issue, agentID, actionClass, command, tier, "allowed_by_policy", reason, policy.ID)
			writeJSON(w, http.StatusOK, map[string]string{"decision": "approved", "tier": tier, "action_class": actionClass})
			return
		}
	}

	// Fall back to a one-time approval (the only path for the security tier).
	latest, err := h.Queries.GetLatestApprovalForAction(r.Context(), db.GetLatestApprovalForActionParams{
		IssueID: issue.ID, ActionKey: actionKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]string{"decision": "none", "tier": tier, "action_class": actionClass})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check approval")
		return
	}
	switch latest.Status {
	case "approved":
		if _, cerr := h.Queries.ConsumeApproval(r.Context(), latest.ID); cerr != nil {
			writeJSON(w, http.StatusOK, map[string]string{"decision": "pending", "tier": tier, "action_class": actionClass})
			return
		}
		h.logApprovalDecision(r.Context(), issue, agentID, actionClass, command, tier, "approved_once", "Allowed by your one-time approval.", pgtype.UUID{})
		writeJSON(w, http.StatusOK, map[string]string{"decision": "approved", "tier": tier, "action_class": actionClass})
	case "consumed":
		writeJSON(w, http.StatusOK, map[string]string{"decision": "none", "tier": tier, "action_class": actionClass})
	default: // pending, rejected
		writeJSON(w, http.StatusOK, map[string]string{"decision": latest.Status, "tier": tier, "action_class": actionClass})
	}
}

// matchPolicy returns the first active learned rule that authorizes actionClass
// for this agent. A policy with a NULL agent_id applies to any agent. Security
// actions never reach here.
func (h *Handler) matchPolicy(ctx context.Context, workspaceID, agentID pgtype.UUID, actionClass string) (db.ApprovalPolicy, bool) {
	policies, err := h.Queries.FindMatchingPolicies(ctx, db.FindMatchingPoliciesParams{
		WorkspaceID: workspaceID, ActionClass: actionClass,
	})
	if err != nil {
		return db.ApprovalPolicy{}, false
	}
	for _, p := range policies {
		// NULL agent_id = any agent; else must match this agent.
		if !p.AgentID.Valid || (agentID.Valid && p.AgentID.Bytes == agentID.Bytes) {
			return p, true
		}
	}
	return db.ApprovalPolicy{}, false
}

// DecideApproval (POST /api/issues/{id}/approvals/{approvalId}/decide) —
// MEMBER-ONLY. Rejects an agent actor so an agent cannot self-approve. Body:
// {"decision":"approve"|"reject"}. On approve, re-triggers the issue so the
// agent resumes (blocked -> todo enqueues via WillEnqueueRun).
func (h *Handler) DecideApproval(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "id")
	issue, ok := h.loadIssueForUser(w, r, issueID)
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := uuidToString(issue.WorkspaceID)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	// The security gate: only a member may decide. An agent token resolves to
	// actorType "agent" and is refused — this is what prevents self-approval.
	if actorType != "member" {
		writeError(w, http.StatusForbidden, "only a member can approve or reject")
		return
	}
	var req struct {
		Decision string `json:"decision"`
		// Learn=true graduates an approval into a reusable policy so this class
		// of action is auto-approved next time ("asked once"). Ignored for
		// security-tier actions — those can never be learned.
		Learn bool `json:"learn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status := ""
	switch req.Decision {
	case "approve":
		status = "approved"
	case "reject":
		status = "rejected"
	default:
		writeError(w, http.StatusBadRequest, "decision must be approve or reject")
		return
	}
	approvalID := chi.URLParam(r, "approvalId")
	decided, err := h.Queries.DecideApproval(r.Context(), db.DecideApprovalParams{
		Status:          status,
		DecidedByMember: parseUUID(userID),
		ID:              parseUUID(approvalID),
		WorkspaceID:     issue.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "approval is not pending (already decided or not found)")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record decision")
		return
	}
	// Approve-and-learn: graduate this approval into a policy so the same class
	// of action auto-approves next time. THE SECURITY FLOOR: only operational
	// actions are learnable — a security-tier command is never turned into a
	// policy, no matter what the member requests.
	if status == "approved" && req.Learn {
		if tier, actionClass := classifyCommand(decided.Command); tier == tierOperational {
			_, _ = h.Queries.CreatePolicy(r.Context(), db.CreatePolicyParams{
				WorkspaceID:      issue.WorkspaceID,
				AgentID:          decided.RequestedByAgent, // scope the rule to the agent that asked
				ActionClass:      actionClass,
				SourceApprovalID: decided.ID,
				CreatedByMember:  parseUUID(userID),
			})
		}
	}
	// On approval, re-trigger the issue so the agent resumes and the hook now
	// allows the action. blocked -> todo enqueues a fresh run (WillEnqueueRun).
	if status == "approved" && issue.Status == "blocked" {
		if updated, uerr := h.Queries.UpdateIssueStatus(r.Context(), db.UpdateIssueStatusParams{
			ID: issue.ID, Status: "todo", WorkspaceID: issue.WorkspaceID,
		}); uerr == nil {
			h.publish(protocol.EventIssueUpdated, workspaceID, actorType, actorID, map[string]any{
				"issue":          issueToResponse(updated, h.getIssuePrefix(r.Context(), issue.WorkspaceID)),
				"status_changed": true, "prev_status": "blocked", "source": "approval_granted",
			})
		}
	}
	writeJSON(w, http.StatusOK, approvalToResponse(decided))
}

// ListPendingApprovals (GET /api/approvals) — member view of everything awaiting
// a decision in the workspace.
func (h *Handler) ListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	rows, err := h.Queries.ListPendingApprovalsForWorkspace(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list approvals")
		return
	}
	resp := make([]approvalResponse, len(rows))
	for i, a := range rows {
		resp[i] = approvalToResponse(a)
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": resp})
}

// ListPolicies (GET /api/approvals/policies) — member view of the learned rules
// (the brain-replica, laid open): every rule with its provenance, active + revoked.
func (h *Handler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	rows, err := h.Queries.ListPolicies(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list policies")
		return
	}
	out := make([]map[string]any, len(rows))
	for i, p := range rows {
		out[i] = map[string]any{
			"id": uuidToString(p.ID), "agent_id": uuidToPtr(p.AgentID),
			"action_class": p.ActionClass, "resource": textToPtr(p.Resource),
			"source_approval_id": uuidToPtr(p.SourceApprovalID),
			"created_at":         timestampToString(p.CreatedAt),
			"revoked":            p.RevokedAt.Valid,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"policies": out})
}

// RevokePolicy (POST /api/approvals/policies/{policyId}/revoke) — MEMBER-ONLY.
// Correctability: stop future auto-approvals from a learned rule.
func (h *Handler) RevokePolicy(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	actorType, _ := h.resolveActor(r, uuidToString(member.UserID), workspaceID)
	if actorType != "member" {
		writeError(w, http.StatusForbidden, "only a member can revoke a policy")
		return
	}
	revoked, err := h.Queries.RevokePolicy(r.Context(), db.RevokePolicyParams{
		RevokedBy:   member.UserID,
		ID:          parseUUID(chi.URLParam(r, "policyId")),
		WorkspaceID: parseUUID(workspaceID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "policy not found or already revoked")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke policy")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": uuidToString(revoked.ID), "revoked": true})
}

// ListDecisions (GET /api/approvals/decisions) — the "why" feed: every gated
// decision + its reasoning, most recent first. This is how the operator asks
// "why did you do that" and traces it back to a rule or a one-time approval.
func (h *Handler) ListDecisions(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	rows, err := h.Queries.ListDecisions(r.Context(), db.ListDecisionsParams{
		WorkspaceID: parseUUID(workspaceID),
		Lim:         100,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list decisions")
		return
	}
	out := make([]map[string]any, len(rows))
	for i, d := range rows {
		out[i] = map[string]any{
			"id": uuidToString(d.ID), "issue_id": uuidToPtr(d.IssueID),
			"action_class": d.ActionClass, "command": d.Command, "tier": d.Tier,
			"decision": d.Decision, "policy_id": uuidToPtr(d.PolicyID),
			"reason": d.Reason, "created_at": timestampToString(d.CreatedAt),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"decisions": out})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// looseTaskUUID parses an optional task-id string into a nullable UUID: an empty
// or malformed value yields an invalid (SQL NULL) UUID rather than panicking.
func looseTaskUUID(s string) pgtype.UUID {
	u, err := parseUUIDLoose(s)
	if err != nil {
		return pgtype.UUID{}
	}
	return u
}
