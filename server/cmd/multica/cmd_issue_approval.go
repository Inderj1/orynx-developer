package main

import (
	"context"
	"fmt"
	"os"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/spf13/cobra"
)

// multica issue approval {request|check|approve|reject} — the human-in-the-loop
// gate for irreversible agent actions (P0.2). The PreToolUse hook uses `check`
// (before an irreversible command) and `request` (when denied); the operator
// uses `approve`/`reject`.
var issueApprovalCmd = &cobra.Command{
	Use:   "approval",
	Short: "Approve or reject an agent's irreversible action",
}

var issueApprovalRequestCmd = &cobra.Command{
	Use:   "request <issue-id>",
	Short: "Record a pending irreversible action and raise an approval request (agent/hook use)",
	Args:  exactArgs(1),
	RunE:  runIssueApprovalRequest,
}

var issueApprovalCheckCmd = &cobra.Command{
	Use:   "check <issue-id>",
	Short: "Check whether an action is approved (prints the decision; consumes an approval)",
	Long: `Prints the decision for an action on this issue: approved | pending | rejected | none.
On "approved" the approval is consumed (single-use). The PreToolUse hook allows the
action only when this prints "approved".`,
	Args: exactArgs(1),
	RunE: runIssueApprovalCheck,
}

var issueApprovalApproveCmd = &cobra.Command{
	Use:   "approve <issue-id> <approval-id>",
	Short: "Approve a pending action (member only) — re-triggers the issue so the agent resumes",
	Args:  exactArgs(2),
	RunE:  func(cmd *cobra.Command, args []string) error { return runIssueApprovalDecide(cmd, args, "approve") },
}

var issueApprovalRejectCmd = &cobra.Command{
	Use:   "reject <issue-id> <approval-id>",
	Short: "Reject a pending action (member only)",
	Args:  exactArgs(2),
	RunE:  func(cmd *cobra.Command, args []string) error { return runIssueApprovalDecide(cmd, args, "reject") },
}

func init() {
	issueApprovalCmd.AddCommand(issueApprovalRequestCmd)
	issueApprovalCmd.AddCommand(issueApprovalCheckCmd)
	issueApprovalCmd.AddCommand(issueApprovalApproveCmd)
	issueApprovalCmd.AddCommand(issueApprovalRejectCmd)

	issueApprovalRequestCmd.Flags().String("command", "", "The exact command being gated (required)")
	issueApprovalRequestCmd.Flags().String("task-id", "", "The task id of the current run (optional)")
	issueApprovalRequestCmd.Flags().String("output", "json", "Output format: table or json")
	issueApprovalCheckCmd.Flags().String("command", "", "The exact command to check")
	issueApprovalCheckCmd.Flags().String("action-key", "", "Precomputed action key (alternative to --command)")
	issueApprovalApproveCmd.Flags().String("output", "json", "Output format: table or json")
	issueApprovalApproveCmd.Flags().Bool("learn", false, "Also learn a rule so this class of action is auto-approved next time (operational actions only; security actions are never learned)")
	issueApprovalRejectCmd.Flags().String("output", "json", "Output format: table or json")

	issueCmd.AddCommand(issueApprovalCmd)
}

func runIssueApprovalRequest(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	issueRef, err := resolveIssueRef(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}
	command, _ := cmd.Flags().GetString("command")
	if command == "" {
		return fmt.Errorf("--command is required")
	}
	taskID, _ := cmd.Flags().GetString("task-id")
	body := map[string]string{"command": command, "task_id": taskID}
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/issues/"+issueRef.ID+"/approvals", body, &result); err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runIssueApprovalCheck(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	issueRef, err := resolveIssueRef(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}
	command, _ := cmd.Flags().GetString("command")
	actionKey, _ := cmd.Flags().GetString("action-key")
	if command == "" && actionKey == "" {
		return fmt.Errorf("--command or --action-key is required")
	}
	body := map[string]string{"command": command, "action_key": actionKey}
	var result struct {
		Decision string `json:"decision"`
	}
	if err := client.PostJSON(ctx, "/api/issues/"+issueRef.ID+"/approvals/check", body, &result); err != nil {
		return err
	}
	// Print just the decision word so the hook can branch on it cheaply.
	fmt.Fprintln(os.Stdout, result.Decision)
	return nil
}

func runIssueApprovalDecide(cmd *cobra.Command, args []string, decision string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	issueRef, err := resolveIssueRef(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}
	approvalID := args[1]
	learn, _ := cmd.Flags().GetBool("learn")
	body := map[string]any{"decision": decision, "learn": learn}
	var result map[string]any
	path := "/api/issues/" + issueRef.ID + "/approvals/" + approvalID + "/decide"
	if err := client.PostJSON(ctx, path, body, &result); err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}
