package commands

import (
	"context"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"strings"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/ghops"
)

// registerActions registers GitHub Actions commands.
func (d *Dispatcher) registerActions() {
	d.Register("actions", Handler{Run: d.cmdActions, HelpText: "List recent workflow runs."})
	d.Register("run", Handler{Run: d.cmdRunWorkflow, HelpText: "Trigger a workflow."})
	d.Register("rerun", Handler{Run: d.cmdRerunWorkflow, HelpText: "Rerun failed jobs (reply to notification)."})
	d.Register("cancel", Handler{Run: d.cmdCancelWorkflow, HelpText: "Cancel a run (reply to notification)."})
	d.Register("logs", Handler{Run: d.cmdWorkflowLogs, HelpText: "Get a link to run logs."})
}

func (d *Dispatcher) cmdActions(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	runs, err := ghops.ListWorkflowRuns(ctx, client, owner, repo, 1, 10)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	if len(runs) == 0 {
		_, _ = d.replyMsg(m, "No recent workflow runs.")
		return nil
	}
	var b strings.Builder
	b.WriteString("<b>Recent Workflow Runs:</b>\n")
	for _, r := range runs {
		fmt.Fprintf(&b, "• #%d %s — <code>%s</code>", r.GetRunNumber(), r.GetName(), r.GetStatus())
		if r.GetConclusion() != "" {
			fmt.Fprintf(&b, " · <code>%s</code>", r.GetConclusion())
		}
		b.WriteString("\n")
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdRunWorkflow(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/run &lt;workflow.yml&gt; [branch]</code>")
		return nil
	}
	workflowFile := args[0]
	branch := "main"
	if len(args) > 1 {
		branch = args[1]
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.DispatchWorkflow(ctx, client, owner, repo, workflowFile, branch, nil); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "actions.dispatch", fmt.Sprintf("%s/%s:%s@%s", owner, repo, workflowFile, branch), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Dispatched workflow <code>%s</code> on <code>%s</code>.", workflowFile, branch))
	return nil
}

func (d *Dispatcher) cmdRerunWorkflow(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, runID, err := d.resolveWorkflowContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.RerunFailedJobs(ctx, client, owner, repo, runID); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "actions.rerun", fmt.Sprintf("%s/%s run %d", owner, repo, runID), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Rerun triggered for run %d.", runID))
	return nil
}

func (d *Dispatcher) cmdCancelWorkflow(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, runID, err := d.resolveWorkflowContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.CancelWorkflowRun(ctx, client, owner, repo, runID); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "actions.cancel", fmt.Sprintf("%s/%s run %d", owner, repo, runID), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Cancelled run %d.", runID))
	return nil
}

func (d *Dispatcher) cmdWorkflowLogs(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, runID, err := d.resolveWorkflowContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	url, err := ghops.GetWorkflowRunLogsURL(ctx, client, owner, repo, runID)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v\n\nNote: logs expire ~10 minutes after a run completes.", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("📥 <a href=\"%s\">Download workflow run logs</a> (expires in ~10 minutes)", url))
	return nil
}

// resolveWorkflowContext tries to extract owner/repo/runID from a reply
// to a workflow_run notification. We don't currently store these in the
// message context DB (only issue/PR contexts are stored), so this falls
// back to requiring the user to specify the run ID.
func (d *Dispatcher) resolveWorkflowContext(ctx context.Context, m *tgbotapi.Message) (string, string, int64, error) {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		return "", "", 0, err
	}
	args := strings.Fields(m.CommandArguments())
	if len(args) < 1 {
		return owner, repo, 0, fmt.Errorf("usage: reply to a workflow notification OR provide the run ID")
	}
	var runID int64
	_, err = fmt.Sscanf(args[0], "%d", &runID)
	if err != nil || runID <= 0 {
		return owner, repo, 0, fmt.Errorf("invalid run ID: %q", args[0])
	}
	return owner, repo, runID, nil
}
