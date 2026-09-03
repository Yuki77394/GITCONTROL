package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/ghops"
	"github.com/swaggymusic/github-bot/internal/telegram"
	"github.com/swaggymusic/github-bot/internal/validation"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// registerPR registers PR management commands.
func (d *Dispatcher) registerPR() {
	d.Register("approve", Handler{Run: d.cmdApprove, HelpText: "Approve a PR (reply to notification)."})
	d.Register("requestchanges", Handler{Run: d.cmdRequestChanges, HelpText: "Request changes on a PR."})
	d.Register("merge", Handler{Run: d.cmdMerge, HelpText: "Merge a PR (with confirmation)."})
	d.Register("draft", Handler{Run: d.cmdDraft, HelpText: "Convert PR to draft (GraphQL-only)."})
	d.Register("ready", Handler{Run: d.cmdReady, HelpText: "Mark draft PR as ready (GraphQL-only)."})
	d.Register("checks", Handler{Run: d.cmdChecks, HelpText: "Show CI status."})
	d.Register("files", Handler{Run: d.cmdFiles, HelpText: "List changed files."})
	d.Register("diff", Handler{Run: d.cmdDiff, HelpText: "Show change summary."})
	d.Register("reviews", Handler{Run: d.cmdReviews, HelpText: "List reviews."})
	d.Register("mergeable", Handler{Run: d.cmdMergeable, HelpText: "Check mergeability."})
	d.Register("request", Handler{Run: d.cmdRequestReview, HelpText: "Request @user as reviewer."})
}

func (d *Dispatcher) cmdApprove(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	body := strings.TrimSpace(strings.TrimPrefix(m.Text, "/approve"))
	if len(args) > 0 {
		body = strings.Join(args, " ")
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.ApprovePR(ctx, client, owner, repo, num, body); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "pr.approve", fmt.Sprintf("%s/%s#%d", owner, repo, num), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ PR #%d approved.", num))
	return nil
}

func (d *Dispatcher) cmdRequestChanges(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	body := strings.TrimSpace(strings.TrimPrefix(m.Text, "/requestchanges"))
	if len(args) > 0 {
		body = strings.Join(args, " ")
	}
	if body == "" {
		body = "Changes requested via SWAGGYMUSIC bot."
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.RequestChanges(ctx, client, owner, repo, num, body); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "pr.request_changes", fmt.Sprintf("%s/%s#%d", owner, repo, num), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Changes requested on PR #%d.", num))
	return nil
}

func (d *Dispatcher) cmdMerge(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	method := ghops.MergeMethodMerge
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "squash":
			method = ghops.MergeMethodSquash
		case "rebase":
			method = ghops.MergeMethodRebase
		case "merge":
			method = ghops.MergeMethodMerge
		default:
			_, _ = d.replyMsg(m, "Usage: <code>/merge [squash|rebase|merge]</code>")
			return nil
		}
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	// Fetch PR for context.
	pr, err := ghops.GetPR(ctx, client, owner, repo, num)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	if !pr.GetMergeable() {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ PR #%d is not mergeable (state: %s). Resolve conflicts first.", num, pr.GetMergeableState()))
		return nil
	}
	// Store action context and ask for confirmation.
	key := PRActionContextKey(m.Chat.ID, int64(m.MessageID))
	d.SetPRActionContext(key, PRActionContext{
		Owner: owner, Repo: repo, PRNumber: num, Method: string(method),
	})
	rows := [][]telegram.Button{
		{{Text: "✅ Confirm merge", Data: "act:merge:" + key}},
		{{Text: "❌ Cancel", Data: "act:cancel:" + key}},
	}
	markup := telegram.InlineKeyboard(rows)
	_, _ = d.deps.Bot.SendMessage(m.Chat.ID,
		fmt.Sprintf("⚠️ <b>Merge confirmation</b>\nPR #%d: %s\nMethod: <code>%s</code>\n\nClick to confirm.", num, pr.GetTitle(), method),
		"HTML", int32(m.MessageID), 0, markup)
	return nil
}

func (d *Dispatcher) cmdDraft(ctx context.Context, m *tgbotapi.Message, args []string) error {
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
	if err := ghops.ConvertPRToDraft(ctx, client, gqc, owner, repo, num); err != nil {
		if errors.Is(err, ghops.ErrUnsupported) {
			_, _ = d.replyMsg(m, "❌ /draft is not supported in this build (no GraphQL client).")
			return nil
		}
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "pr.draft", fmt.Sprintf("%s/%s#%d", owner, repo, num), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ PR #%d converted to draft.", num))
	return nil
}

func (d *Dispatcher) cmdReady(ctx context.Context, m *tgbotapi.Message, args []string) error {
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
	if err := ghops.MarkPRReady(ctx, client, gqc, owner, repo, num); err != nil {
		if errors.Is(err, ghops.ErrUnsupported) {
			_, _ = d.replyMsg(m, "❌ /ready is not supported in this build (no GraphQL client).")
			return nil
		}
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "pr.ready", fmt.Sprintf("%s/%s#%d", owner, repo, num), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ PR #%d marked ready for review.", num))
	return nil
}

func (d *Dispatcher) cmdChecks(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	pr, err := ghops.GetPR(ctx, client, owner, repo, num)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	runs, err := ghops.ListChecks(ctx, client, owner, repo, pr.GetHead().GetSHA())
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	if len(runs) == 0 {
		_, _ = d.replyMsg(m, "No check runs found for the PR head SHA.")
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Checks for PR #%d:</b>\n", num)
	for _, r := range runs {
		fmt.Fprintf(&b, "• %s — <code>%s</code>", r.GetName(), r.GetStatus())
		if r.GetConclusion() != "" {
			fmt.Fprintf(&b, " · <code>%s</code>", r.GetConclusion())
		}
		b.WriteString("\n")
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdFiles(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	files, err := ghops.ListPRFiles(ctx, client, owner, repo, num)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Changed files (%d):</b>\n", len(files))
	for i, f := range files {
		if i >= 20 {
			fmt.Fprintf(&b, "… and %d more\n", len(files)-20)
			break
		}
		fmt.Fprintf(&b, "• <code>%s</code> +%d/-%d\n", f.GetFilename(), f.GetAdditions(), f.GetDeletions())
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdDiff(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	pr, err := ghops.GetPR(ctx, client, owner, repo, num)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("<b>PR #%d diff summary</b>\nFiles: %d\n+%d / -%d\nCommits: %d",
		num, pr.GetChangedFiles(), pr.GetAdditions(), pr.GetDeletions(), pr.GetCommits()))
	return nil
}

func (d *Dispatcher) cmdReviews(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	list, err := ghops.ListPRReviews(ctx, client, owner, repo, num)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Reviews on PR #%d:</b>\n", num)
	for _, r := range list {
		fmt.Fprintf(&b, "• %s — <code>%s</code>\n", r.GetUser().GetLogin(), r.GetState())
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdMergeable(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	ok, err := ghops.PRMergeable(ctx, client, owner, repo, num)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	if ok {
		_, _ = d.replyMsg(m, fmt.Sprintf("✅ PR #%d is mergeable.", num))
	} else {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ PR #%d is NOT mergeable.", num))
	}
	return nil
}

func (d *Dispatcher) cmdRequestReview(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, num, err := d.resolveIssueContext(ctx, m)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/request @username</code>")
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
	if err := ghops.RequestReviewers(ctx, client, owner, repo, num, []string{user}); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Requested review from @%s on PR #%d.", user, num))
	return nil
}
