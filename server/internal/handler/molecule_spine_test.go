package handler

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TestMoleculeSpine_IsBlockedRecompute verifies the dependency DAG + is_blocked
// recompute + ready-work read (migrations 203-207, queries/issue_dependency.sql):
//   - a 'blocks' edge blocks the dependent while the blocker is active,
//   - blocked-ness propagates DOWN the epic tree (a child of a blocked issue),
//   - closing the blocker unblocks the dependents,
//   - ready-work excludes blocked issues.
//
// Runs against the CI database; skips when none is available (same contract as
// the other DB-backed handler tests).
func TestMoleculeSpine_IsBlockedRecompute(t *testing.T) {
	if testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	q := db.New(testPool)

	// A dedicated workspace so the workspace-wide recompute only touches our fixtures.
	wsID, userID := setupSpineFixture(ctx, t, testPool)
	defer teardownSpineFixture(ctx, testPool, wsID, userID)
	ws := parseUUID(wsID)

	// A=blocker; B depends-on A. X=blocker; E(epic) depends-on X; C=child of E. R=independent.
	a := insertSpineIssue(ctx, t, testPool, wsID, userID, "A-blocker", "todo", "", 1)
	b := insertSpineIssue(ctx, t, testPool, wsID, userID, "B-dependent", "todo", "", 2)
	x := insertSpineIssue(ctx, t, testPool, wsID, userID, "X-blocker", "in_progress", "", 3)
	e := insertSpineIssue(ctx, t, testPool, wsID, userID, "E-epic", "todo", "", 4)
	c := insertSpineIssue(ctx, t, testPool, wsID, userID, "C-child", "todo", e, 5)
	r := insertSpineIssue(ctx, t, testPool, wsID, userID, "R-ready", "todo", "", 6)

	mustDep := func(dependent, blocker string) {
		t.Helper()
		if _, err := q.CreateDependency(ctx, db.CreateDependencyParams{
			WorkspaceID: ws, IssueID: parseUUID(dependent), DependsOnID: parseUUID(blocker), DepType: "blocks",
		}); err != nil {
			t.Fatalf("CreateDependency: %v", err)
		}
	}
	mustDep(b, a)
	mustDep(e, x)

	// Recompute #1: blockers active -> B, E, and (by propagation) C are blocked.
	if err := q.RecomputeWorkspaceBlocked(ctx, ws); err != nil {
		t.Fatalf("recompute #1: %v", err)
	}
	assertBlocked(ctx, t, testPool, map[string]bool{
		a: false, b: true, x: false, e: true, c: true, r: false,
	})

	// Ready-work excludes the blocked issues, includes the unblocked active ones.
	ready, err := q.GetReadyIssues(ctx, db.GetReadyIssuesParams{WorkspaceID: ws, Lim: 100})
	if err != nil {
		t.Fatalf("GetReadyIssues: %v", err)
	}
	readySet := map[string]bool{}
	for _, iss := range ready {
		readySet[uuidToString(iss.ID)] = true
	}
	for _, id := range []string{a, x, r} {
		if !readySet[id] {
			t.Errorf("ready-work should include unblocked issue %s", id)
		}
	}
	for _, id := range []string{b, e, c} {
		if readySet[id] {
			t.Errorf("ready-work must exclude blocked issue %s", id)
		}
	}

	// Close the blockers -> the whole graph unblocks.
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status='done' WHERE id IN ($1, $2)`, a, x); err != nil {
		t.Fatalf("close blockers: %v", err)
	}
	if err := q.RecomputeWorkspaceBlocked(ctx, ws); err != nil {
		t.Fatalf("recompute #2: %v", err)
	}
	assertBlocked(ctx, t, testPool, map[string]bool{
		b: false, e: false, c: false, r: false,
	})
}

func setupSpineFixture(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	// Clear leftovers from a prior failed run (no FK from workspace to our deps).
	_, _ = pool.Exec(ctx, `DELETE FROM workspace WHERE slug = 'spine-dag-test'`)
	_, _ = pool.Exec(ctx, `DELETE FROM "user" WHERE email = 'spine-dag-test@multica.ai'`)

	var userID, wsID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO "user" (name, email) VALUES ('Spine Test', 'spine-dag-test@multica.ai') RETURNING id`,
	).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO workspace (name, slug, description, issue_prefix)
		 VALUES ('Spine DAG Test', 'spine-dag-test', '', 'SPN') RETURNING id`,
	).Scan(&wsID); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, wsID, userID,
	); err != nil {
		t.Fatalf("create member: %v", err)
	}
	return wsID, userID
}

func teardownSpineFixture(ctx context.Context, pool *pgxpool.Pool, wsID, userID string) {
	// issue_dependency has no FK to workspace, so clean it explicitly first.
	_, _ = pool.Exec(ctx, `DELETE FROM issue_dependency WHERE workspace_id = $1`, wsID)
	_, _ = pool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, wsID) // cascades issue, member
	_, _ = pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, userID)
}

// insertSpineIssue inserts a minimal issue and returns its id. parentID "" => NULL.
// num sets the per-workspace issue number (unique constraint uq_issue_workspace_number),
// which the normal create path assigns; raw inserts must supply distinct values.
func insertSpineIssue(ctx context.Context, t *testing.T, pool *pgxpool.Pool, wsID, userID, title, status, parentID string, num int32) string {
	t.Helper()
	var parent any
	if parentID != "" {
		parent = parentID
	}
	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO issue (workspace_id, title, status, priority, creator_type, creator_id, parent_issue_id, number)
		 VALUES ($1, $2, $3, 'none', 'member', $4, $5, $6) RETURNING id`,
		wsID, title, status, userID, parent, num,
	).Scan(&id); err != nil {
		t.Fatalf("insert issue %q: %v", title, err)
	}
	return id
}

func assertBlocked(ctx context.Context, t *testing.T, pool *pgxpool.Pool, want map[string]bool) {
	t.Helper()
	for id, exp := range want {
		var got bool
		if err := pool.QueryRow(ctx, `SELECT is_blocked FROM issue WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatalf("read is_blocked %s: %v", id, err)
		}
		if got != exp {
			t.Errorf("issue %s: is_blocked = %v, want %v", id, got, exp)
		}
	}
}
