package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/ghops"
	"github.com/swaggymusic/github-bot/internal/validation"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	gh "github.com/google/go-github/v66/github"
)

// registerIssue registers issue-management commands.
func (d *Dispatcher) registerIssue() {
	d.Register("issue", Handler{Run: d.cmdIssue, HelpText: "Create an issue."})
	d.Register("comment", Handler{Run: d.cmdComment, HelpText: "Comment on a replied issue/PR."})
	d.Register("close", Handler{Run: d.cmdClose, HelpText: "Close issue/PR (reply to notification)."})
	d.Register("reopen", Handler{Run: d.cmdReopen, HelpText: "Reopen issue/PR."})
	d.Register("assign", Handler{Run: d.cmdAssign, HelpText: "Assign @user."})
	d.Register("assignme", Handler{Run: d.cmdAssignMe, HelpText: "Assign yourself."})
	d.Register("unassign", Handler{Run: d.cmdUnassign, HelpText: "Unassign @user."})
	d.Register("label", Handler{Run: d.cmdLabel, HelpText: "Add/remove labels."})
	d.Register("labels", Handler{Run: d.cmdLabels, HelpText: "List labels."})
	d.Register("milestone", Handler{Run: d.cmdMilestone, HelpText: "Set milestone."})
	d.Register("lock", Handler{Run: d.cmdLock, HelpText: "Lock conversation."})
	d.Register("unlock", Handler{Run: d.cmdUnlock, HelpText: "Unlock conversation."})
	d.Register("pin", Handler{Run: d.cmdPin, HelpText: "Pin issue (GraphQL-only, may be unsupported)."})
	d.Register("unpin", Handler{Run: d.cmdUnpin, HelpText: "Unpin issue (GraphQL-only)."})
}

// resolveIssueContext returns (owner, repo, issueNumber, error). The issue
// number is taken from reply context. Used by reply-style commands.
func (d *Dispatcher) resolveIssueContext(ctx context.Context, m *tgbotapi.Message) (string, string, int, error) {
	if m.ReplyToMessage == nil {
		return "", "", 0, fmt.Errorf("reply to an issue/PR notification to use this command")
	}
	mc, err := d.deps.DB.GetMessageContext(ctx, m.Chat.ID, int64(m.ReplyToMessage.MessageID))
	if err != nil || mc == nil {
		return "", "", 0, fmt.Errorf("no issue/PR context for the replied message")
	}
	return mc.Owner, mc.Repo, mc.IssueNumber, nil
}

func (d *Dispatcher) cmdIssue(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	raw := strings.TrimSpace(strings.TrimPrefix(m.Text, "/issue"))
	raw = strings.TrimSpace(strings.TrimPrefix(raw, m.Command()))
	if len(args) > 0 {
		raw = strings.TrimSpace(strings.Join(args, " "))
	}
	if raw == "" {
		_, _ = d.replyMsg(m, "Usage: <code>/issue &lt;title&gt;</code>\nOptionally add a body on a new line.")
		return nil
	}
	// Split into title + body on first newline.
	title := raw
	body := ""
	if idx := strings.Index(raw, "\n"); idx >= 0 {
		title = strings.TrimSpace(raw[:idx])
		body = validation.SanitizeText(raw[idx+1:], 60000)
	}
	title = validation.SanitizeText(title, 256)
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	issue, err := ghops.CreateIssue(ctx, client, owner, repo, title, body, nil, nil)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "issue.create", fmt.Sprintf("%s/%s#%d", owner, repo, issue.GetNumber()), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Issue #%d created: <a href=\"%s\">%s</a>", issue.GetNumber(), issue.GetHTMLURL(), title))
	return nil
}

func (d *Dispatcher) cmdComment(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	body := strings.TrimSpace(strings.TrimPrefix(m.Text, "/comment"))
	if body == "" {
		_, _ = d.replyMsg(m, "Usage: reply to an issue/PR notification with <code>/comment &lt;text&gt;</code>")
		return nil
	}
	body = validation.SanitizeText(body, 60000)
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if _, err := ghops.CommentIssue(ctx, client, owner, repo, num, body); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, "✅ Comment posted.")
	return nil
}

func (d *Dispatcher) cmdClose(ctx context.Context, m *tgbotapi.Message, args []string) error {
	return d.issueStateAction(ctx, m, "close")
}

func (d *Dispatcher) cmdReopen(ctx context.Context, m *tgbotapi.Message, args []string) error {
	return d.issueStateAction(ctx, m, "reopen")
}

func (d *Dispatcher) issueStateAction(ctx context.Context, m *tgbotapi.Message, action string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if action == "close" {
		err = ghops.CloseIssue(ctx, client, owner, repo, num)
	} else {
		err = ghops.ReopenIssue(ctx, client, owner, repo, num)
	}
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "issue."+action, fmt.Sprintf("%s/%s#%d", owner, repo, num), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ %s/%s#%d %sed.", owner, repo, num, action))
	return nil
}

func (d *Dispatcher) cmdAssign(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/assign @username</code>")
		return nil
	}
	user := validation.NormalizeUsername(args[0])
	if err := validation.ValidateGitHubUsername(user); err != nil {
		_, _ = d.replyMsg(m, "❌ Invalid username.")
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.AssignUsers(ctx, client, owner, repo, num, []string{user}); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Assigned @%s to #%d.", user, num))
	return nil
}

func (d *Dispatcher) cmdAssignMe(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	// Get the user's GitHub username from their connected account.
	_, acc, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	user := acc.GitHubUsername
	if user == "" {
		_, _ = d.replyMsg(m, "❌ Could not determine your GitHub username.")
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.AssignUsers(ctx, client, owner, repo, num, []string{user}); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Assigned @%s to #%d.", user, num))
	return nil
}

func (d *Dispatcher) cmdUnassign(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/unassign @username</code>")
		return nil
	}
	user := validation.NormalizeUsername(args[0])
	if err := validation.ValidateGitHubUsername(user); err != nil {
		_, _ = d.replyMsg(m, "❌ Invalid username.")
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.UnassignUsers(ctx, client, owner, repo, num, []string{user}); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Unassigned @%s from #%d.", user, num))
	return nil
}

func (d *Dispatcher) cmdLabel(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/label +bug -wip</code>")
		return nil
	}
	adds, removes, err := validation.ParseLabelArgs(args)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if len(adds) > 0 {
		if err := ghops.AddLabels(ctx, client, owner, repo, num, adds); err != nil {
			_, _ = d.replyMsg(m, fmt.Sprintf("❌ add labels: %v", err))
			return nil
		}
	}
	for _, l := range removes {
		if err := ghops.RemoveLabel(ctx, client, owner, repo, num, l); err != nil {
			// Don't fail hard — some labels may not be present.
			_ = err
		}
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Labels updated on #%d.", num))
	return nil
}

func (d *Dispatcher) cmdLabels(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	list, err := ghops.ListLabels(ctx, client, owner, repo, 1, 20)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	b.WriteString("<b>Labels:</b>\n")
	for _, l := range list {
		fmt.Fprintf(&b, "• <code>%s</code>", l.GetName())
		if l.GetDescription() != "" {
			fmt.Fprintf(&b, " — %s", l.GetDescription())
		}
		b.WriteString("\n")
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdMilestone(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/milestone &lt;name&gt;</code>")
		return nil
	}
	// Milestone names need to be resolved to numbers via the API.
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	name := strings.Join(args, " ")
	list, _, err := client.Issues.ListMilestones(ctx, owner, repo, &gh.MilestoneListOptions{ListOptions: gh.ListOptions{PerPage: 100}})
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ list milestones: %v", err))
		return nil
	}
	for _, ms := range list {
		if strings.EqualFold(ms.GetTitle(), name) {
			if err := ghops.SetMilestone(ctx, client, owner, repo, num, int(ms.GetNumber())); err != nil {
				_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
				return nil
			}
			_, _ = d.replyMsg(m, fmt.Sprintf("✅ Milestone set to <b>%s</b> on #%d.", ms.GetTitle(), num))
			return nil
		}
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("❌ Milestone <b>%s</b> not found in %s/%s.", name, owner, repo))
	return nil
}

func (d *Dispatcher) cmdLock(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	reason := "off-topic"
	if len(args) > 0 {
		reason = args[0]
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.LockIssue(ctx, client, owner, repo, num, reason); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ #%d locked.", num))
	return nil
}

func (d *Dispatcher) cmdUnlock(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.UnlockIssue(ctx, client, owner, repo, num); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ #%d unlocked.", num))
	return nil
}

func (d *Dispatcher) cmdPin(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	gqc, err := d.newGraphQLClient(ctx, m.From.ID)
	if err != nil {
		_, _ = d.replyMsg(m, "⚠️ Could not build GraphQL client (token decryption failed). Use /replacetoken.")
		return nil
	}
	if err := ghops.PinIssue(ctx, client, gqc, owner, repo, num); err != nil {
		if errors.Is(err, ghops.ErrUnsupported) {
			_, _ = d.replyMsg(m, "❌ /pin is not supported in this build (no GraphQL client).")
			return nil
		}
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Pinning failed: %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "issue.pin", fmt.Sprintf("%s/%s#%d", owner, repo, num), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Issue #%d pinned.", num))
	return nil
}

func (d *Dispatcher) cmdUnpin(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	gqc, err := d.newGraphQLClient(ctx, m.From.ID)
	if err != nil {
		_, _ = d.replyMsg(m, "⚠️ Could not build GraphQL client (token decryption failed). Use /replacetoken.")
		return nil
	}
	if err := ghops.UnpinIssue(ctx, client, gqc, owner, repo, num); err != nil {
		if errors.Is(err, ghops.ErrUnsupported) {
			_, _ = d.replyMsg(m, "❌ /unpin is not supported in this build (no GraphQL client).")
			return nil
		}
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Unpinning failed: %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "issue.unpin", fmt.Sprintf("%s/%s#%d", owner, repo, num), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Issue #%d unpinned.", num))
	return nil
}
