package handler

import (
	"context"
	"testing"
)

// knownNonCascadeWorkspaceTables lists every table carrying a workspace_id
// column that does NOT have an ON DELETE CASCADE foreign key directly to the
// workspace table. Rows in these tables are NOT removed implicitly when a
// workspace is deleted, so each must be handled explicitly (swept by the
// DeleteWorkspace CTE), cascade transitively through a workspace-scoped parent,
// or be a deliberate retention.
//
// This is the integrity guard from the platform review: the codebase runs two
// integrity models (legacy FK cascades vs. app-code cleanup) with no
// enforcement, so a new workspace-scoped table someone forgets to wire into the
// delete path orphans rows silently. This test forces a conscious decision —
// adding such a table fails the test until it is listed here with a reason AND
// its cleanup path is wired up.
//
// Entries marked "(verify)" were flagged by the review as candidates that may
// not currently be cleaned up; they are allow-listed to keep the guard green
// while their cleanup paths are confirmed, not as an assertion that they are
// safe.
var knownNonCascadeWorkspaceTables = map[string]string{
	// Explicitly swept by the DeleteWorkspace CTE.
	"channel_binding_token":      "swept by the DeleteWorkspace CTE",
	"channel_installation":       "swept by the DeleteWorkspace CTE",
	"channel_user_binding":       "swept by the DeleteWorkspace CTE",
	"github_pending_check_suite": "swept by the DeleteWorkspace CTE",
	"issue_property":             "swept by the DeleteWorkspace CTE (custom property defs)",

	// Swept explicitly in the DeleteWorkspace handler transaction (own query,
	// no FK to workspace).
	"chat_pinned_agent":      "swept by DeleteChatPinnedAgentsByWorkspace in the delete tx",
	"runtime_profile":        "swept by DeleteRuntimeProfilesByWorkspace in the delete tx",
	"autopilot_rule_version": "swept by DeleteAutopilotRuleVersionsByWorkspace in the delete tx",
	"issue_dependency":       "swept by DeleteDependenciesByWorkspace in the delete tx",
	"issue_approval":         "swept by DeleteIssueApprovalsByWorkspace in the delete tx",
	"approval_policy":        "swept by DeleteApprovalPoliciesByWorkspace in the delete tx",
	"approval_decision_log":  "swept by DeleteApprovalDecisionLogByWorkspace in the delete tx",

	// Handled by the schema without a direct workspace cascade.
	"feedback":          "retained: workspace_id SET NULL on workspace delete (feedback outlives the workspace)",
	"lark_user_binding": "cascades transitively via lark_installation -> workspace (both ON DELETE CASCADE)",

	// Deliberately retained, unbounded usage analytics — not cleaned on
	// workspace delete by design. Flagged for a future retention/GC policy.
	"task_usage_hourly":       "retained: usage analytics (no cleanup by design; unbounded)",
	"task_usage_hourly_dirty": "retained: usage-analytics dirty marker (no cleanup by design)",
}

// TestWorkspaceScopedTablesHaveDeleteCoverage asserts every workspace_id table
// either cascades from workspace or is a reviewed entry in the allowlist above.
func TestWorkspaceScopedTablesHaveDeleteCoverage(t *testing.T) {
	if testPool == nil {
		t.Skip("no database")
	}
	ctx := context.Background()
	rows, err := testPool.Query(ctx, `
		SELECT c.table_name
		FROM information_schema.columns c
		WHERE c.column_name = 'workspace_id' AND c.table_schema = 'public'
		  AND NOT EXISTS (
		    SELECT 1 FROM pg_constraint con
		    JOIN pg_class rel  ON rel.oid  = con.conrelid
		    JOIN pg_class frel ON frel.oid = con.confrelid
		    WHERE con.contype = 'f' AND rel.relname = c.table_name
		      AND frel.relname = 'workspace' AND con.confdeltype = 'c'
		  )
		ORDER BY c.table_name`)
	if err != nil {
		t.Fatalf("schema query failed: %v", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var tbl string
		if err := rows.Scan(&tbl); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen[tbl] = true
		if _, ok := knownNonCascadeWorkspaceTables[tbl]; !ok {
			t.Errorf("table %q has a workspace_id column, no ON DELETE CASCADE to workspace, and is not in knownNonCascadeWorkspaceTables. "+
				"Wire its cleanup into DeleteWorkspace (or document why it is retained) and add it to the allowlist.", tbl)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	// Keep the allowlist from rotting: a listed table that no longer matches
	// (dropped / renamed / gained a cascade) should be removed.
	for tbl := range knownNonCascadeWorkspaceTables {
		if !seen[tbl] {
			t.Errorf("allowlist entry %q no longer matches a non-cascade workspace_id table — remove it from knownNonCascadeWorkspaceTables.", tbl)
		}
	}
}
