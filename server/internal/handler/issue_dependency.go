package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Issue dependency-edge API (molecule spine, Phase 1). Edges are always stored;
// is_blocked is only recomputed when the spine is enabled (MOLECULE_SPINE=1),
// since only then does dispatch consult it. Mirrors the service-side flag.
func moleculeSpineOn() bool { return os.Getenv("MOLECULE_SPINE") == "1" }

// ListIssueDependencies returns what an issue depends on (its blockers) and what
// depends on it (its dependents).
func (h *Handler) ListIssueDependencies(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	blockers, err := h.Queries.ListDependenciesForIssue(r.Context(), db.ListDependenciesForIssueParams{
		WorkspaceID: issue.WorkspaceID, IssueID: issue.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list dependencies")
		return
	}
	dependents, err := h.Queries.ListDependentsOf(r.Context(), db.ListDependentsOfParams{
		WorkspaceID: issue.WorkspaceID, DependsOnID: issue.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list dependents")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"depends_on": dependencyRows(blockers),
		"dependents": dependencyRows(dependents),
	})
}

func dependencyRows(rows []db.IssueDependency) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, d := range rows {
		out[i] = map[string]any{
			"id":            uuidToString(d.ID),
			"issue_id":      uuidToString(d.IssueID),
			"depends_on_id": uuidToString(d.DependsOnID),
			"dep_type":      d.DepType,
		}
	}
	return out
}

// CreateIssueDependency adds an edge: this issue (the {id}) depends on
// depends_on_id. Both endpoints are resolved through loadIssueForUser, which
// enforces the requester's workspace, so a cross-workspace edge is impossible.
func (h *Handler) CreateIssueDependency(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req struct {
		DependsOnID string `json:"depends_on_id"`
		DepType     string `json:"dep_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DependsOnID == "" {
		writeError(w, http.StatusBadRequest, "depends_on_id is required")
		return
	}
	blocker, ok := h.loadIssueForUser(w, r, req.DependsOnID)
	if !ok {
		return
	}
	if blocker.ID == issue.ID {
		writeError(w, http.StatusBadRequest, "an issue cannot depend on itself")
		return
	}
	depType := "blocks"
	if req.DepType == "related" {
		depType = "related"
	}
	dep, err := h.Queries.CreateDependency(r.Context(), db.CreateDependencyParams{
		WorkspaceID: issue.WorkspaceID,
		IssueID:     issue.ID,
		DependsOnID: blocker.ID,
		DepType:     depType,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create dependency")
		return
	}
	h.recomputeSpineBlockedIfOn(r, issue.WorkspaceID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":            uuidToString(dep.ID),
		"issue_id":      uuidToString(dep.IssueID),
		"depends_on_id": uuidToString(dep.DependsOnID),
		"dep_type":      dep.DepType,
	})
}

// DeleteIssueDependency removes the edge (this issue depends on dependsOnId).
func (h *Handler) DeleteIssueDependency(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	blocker, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "dependsOnId"))
	if !ok {
		return
	}
	if err := h.Queries.DeleteDependency(r.Context(), db.DeleteDependencyParams{
		WorkspaceID: issue.WorkspaceID,
		IssueID:     issue.ID,
		DependsOnID: blocker.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete dependency")
		return
	}
	h.recomputeSpineBlockedIfOn(r, issue.WorkspaceID)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// recomputeSpineBlockedIfOn refreshes is_blocked for the workspace when the spine
// is enabled. Best-effort: a lost recompute self-corrects on the next change.
func (h *Handler) recomputeSpineBlockedIfOn(r *http.Request, workspaceID pgtype.UUID) {
	if !moleculeSpineOn() {
		return
	}
	if err := h.Queries.RecomputeWorkspaceBlocked(r.Context(), workspaceID); err != nil {
		slog.Warn("molecule spine: recompute blocked failed", "error", err)
	}
}
