package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/spf13/cobra"
)

// multica task nudge {send|check} — mid-run steering (Phase 2). `send` queues a
// steering message for a running task (operator use); `check` consumes queued
// nudges for a task and prints them (the agent's Stop hook uses this at a turn
// boundary to continue the run with the steer instead of stopping).
var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Task operations",
}

var taskNudgeCmd = &cobra.Command{
	Use:   "nudge",
	Short: "Mid-run steering for a running task",
}

var taskNudgeSendCmd = &cobra.Command{
	Use:   "send <task-id> <message>",
	Short: "Queue a steering message for a running task (redirects it without cancelling)",
	Args:  exactArgs(2),
	RunE:  runTaskNudgeSend,
}

var taskNudgeCheckCmd = &cobra.Command{
	Use:   "check <task-id>",
	Short: "Consume queued nudges for a task and print them (used by the agent Stop hook)",
	Args:  exactArgs(1),
	RunE:  runTaskNudgeCheck,
}

func init() {
	taskNudgeCmd.AddCommand(taskNudgeSendCmd)
	taskNudgeCmd.AddCommand(taskNudgeCheckCmd)
	taskCmd.AddCommand(taskNudgeCmd)
	rootCmd.AddCommand(taskCmd)

	taskNudgeSendCmd.Flags().String("output", "json", "Output format: table or json")
}

func runTaskNudgeSend(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	body := map[string]string{"content": args[1]}
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/tasks/"+args[0]+"/nudge", body, &result); err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runTaskNudgeCheck(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result struct {
		Nudges []string `json:"nudges"`
	}
	if err := client.PostJSON(ctx, "/api/tasks/"+args[0]+"/nudges/consume", map[string]string{}, &result); err != nil {
		return err
	}
	// Print the consumed nudges joined for the Stop hook to inject verbatim;
	// empty output means no queued nudges.
	if len(result.Nudges) > 0 {
		fmt.Fprintln(os.Stdout, strings.Join(result.Nudges, "\n\n"))
	}
	return nil
}
