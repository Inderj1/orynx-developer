package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Mid-run steering (Phase 2). A member queues a "nudge" for a running task; the
// agent's Stop hook consumes queued nudges at a turn boundary and continues with
// them, so an operator can redirect a run without cancelling it.

// SendNudge queues a steering message for a running task (member action).
func (h *Handler) SendNudge(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, ctxWorkspaceID(r.Context()), "workspace id")
	if !ok {
		return
	}
	taskUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "taskId"), "task id")
	if !ok {
		return
	}
	task, err := h.Queries.GetAgentTaskInWorkspace(r.Context(), db.GetAgentTaskInWorkspaceParams{
		ID: taskUUID, WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	// Steering only applies to a task that is still in flight.
	switch task.Status {
	case "queued", "dispatched", "running":
	default:
		writeError(w, http.StatusConflict, "task is not running")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	nudge, err := h.Queries.CreateNudge(r.Context(), db.CreateNudgeParams{
		WorkspaceID:  wsUUID,
		TaskID:       taskUUID,
		Content:      strings.TrimSpace(req.Content),
		AuthorMember: parseUUID(userID),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue nudge")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": uuidToString(nudge.ID), "status": nudge.Status,
	})
}

// ConsumeNudges atomically returns + marks-delivered the queued nudges for a
// task. Called by the agent's Stop hook (agent/daemon token) at a turn boundary.
func (h *Handler) ConsumeNudges(w http.ResponseWriter, r *http.Request) {
	wsUUID, ok := parseUUIDOrBadRequest(w, ctxWorkspaceID(r.Context()), "workspace id")
	if !ok {
		return
	}
	taskUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "taskId"), "task id")
	if !ok {
		return
	}
	// Scope the consume to a task that actually belongs to the token's workspace.
	if _, err := h.Queries.GetAgentTaskInWorkspace(r.Context(), db.GetAgentTaskInWorkspaceParams{
		ID: taskUUID, WorkspaceID: wsUUID,
	}); err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	nudges, err := h.Queries.ConsumeQueuedNudges(r.Context(), db.ConsumeQueuedNudgesParams{
		WorkspaceID: wsUUID, TaskID: taskUUID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to consume nudges")
		return
	}
	if nudges == nil {
		nudges = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"nudges": nudges})
}
