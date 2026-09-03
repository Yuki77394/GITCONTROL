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
)

// registerDiscussions registers GitHub Discussions commands.
//
// Discussions are GitHub-GraphQL-only. They require:
//   - The repository to have Discussions enabled.
//   - The user's token to have the `read:discussion` scope (for list/view)
//     and `write:discussion` scope (for create / mark answer).
//
// If the repository does not have Discussions enabled, the GraphQL query
// will return an error which we surface to the user.
func (d *Dispatcher) registerDiscussions() {
	d.Register("discussion", Handler{Run: d.cmdDiscussion, HelpText: "Create or list discussions."})
	d.Register("discussions", Handler{Run: d.cmdListDiscussions, HelpText: "List discussions."})
	d.Register("answered", Handler{Run: d.cmdAnswered, HelpText: "Mark a discussion comment as the answer (reply to notification)."})
}

// cmdListDiscussions lists recent discussions in the current repo context.
func (d *Dispatcher) cmdListDiscussions(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	gqc, err := d.newGraphQLClient(ctx, m.From.ID)
	if err != nil {
		_, _ = d.replyMsg(m, "⚠️ Could not build GraphQL client. Use /replacetoken.")
		return nil
	}
	list, err := gqc.ListDiscussions(ctx, owner, repo, 10)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v\n\nNote: Discussions must be enabled on the repository, and your token needs the `read:discussion` scope.", err))
		return nil
	}
	if len(list) == 0 {
		_, _ = d.replyMsg(m, "No discussions found.")
		return nil
	}
	var b strings.Builder
	b.WriteString("<b>Recent Discussions:</b>\n")
	for _, disc := range list {
		fmt.Fprintf(&b, "• #%d %s", disc.Number, disc.Title)
		if disc.IsAnswered {
			b.WriteString(" ✅")
		}
		b.WriteString("\n")
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

// cmdDiscussion creates a new discussion.
//
// Usage:
//
//	/discussion <title>
//	/discussion <title> <body>     (body on same line)
//
// The category is required by GitHub's GraphQL API. Since selecting a
// category interactively is cumbersome in Telegram, we default to the first
// category returned by the repository. If the repository has no categories
// (Discussions not enabled), the command fails with a clear error.
func (d *Dispatcher) cmdDiscussion(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	raw := strings.TrimSpace(m.CommandArguments())
	if raw == "" {
		// No args → list discussions instead.
		return d.cmdListDiscussions(ctx, m, args)
	}
	// Split title/body on first newline.
	title := raw
	body := ""
	if idx := strings.Index(raw, "\n"); idx >= 0 {
		title = strings.TrimSpace(raw[:idx])
		body = validation.SanitizeText(raw[idx+1:], 60000)
	}
	title = validation.SanitizeText(title, 256)
	if title == "" {
		_, _ = d.replyMsg(m, "Usage: <code>/discussion &lt;title&gt;</code>\nOptionally add a body on a new line.")
		return nil
	}
	gqc, err := d.newGraphQLClient(ctx, m.From.ID)
	if err != nil {
		_, _ = d.replyMsg(m, "⚠️ Could not build GraphQL client. Use /replacetoken.")
		return nil
	}
	// Fetch the repo's Node ID and the first discussion category.
	repoNodeID, err := gqc.GetRepoNodeID(ctx, owner, repo)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	categories, err := gqc.ListDiscussionCategories(ctx, owner, repo)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v\n\nNote: Discussions must be enabled on the repository.", err))
		return nil
	}
	if len(categories) == 0 {
		_, _ = d.replyMsg(m, "❌ Repository has no discussion categories. Enable Discussions in repository settings.")
		return nil
	}
	// Default to the first category (GitHub usually returns "General" or "Q&A" first).
	category := categories[0]
	url, err := gqc.CreateDiscussion(ctx, repoNodeID, category.ID, title, body)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "discussion.create", fmt.Sprintf("%s/%s:%s", owner, repo, title), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Discussion created in <b>%s</b>:\n<a href=\"%s\">%s</a>", category.Name, url, title))
	return nil
}

// cmdAnswered marks a discussion comment as the answer.
//
// This command must be used as a reply to a discussion-comment notification.
// The bot looks up the stored MessageContext to find the discussion number
// and comment ID, then calls the GraphQL markDiscussionCommentAsAnswer
// mutation.
func (d *Dispatcher) cmdAnswered(ctx context.Context, m *tgbotapi.Message, args []string) error {
	if m.ReplyToMessage == nil {
		_, _ = d.replyMsg(m, "Reply to a discussion comment notification to use /answered.")
		return nil
	}
	mc, err := d.deps.DB.GetMessageContext(ctx, m.Chat.ID, int64(m.ReplyToMessage.MessageID))
	if err != nil || mc == nil {
		_, _ = d.replyMsg(m, "❌ No discussion context found for the replied message.")
		return nil
	}
	// /answered only works on discussion_comment message contexts.
	if mc.Type != "discussion_comment" {
		_, _ = d.replyMsg(m, "❌ The replied message is not a discussion comment.")
		return nil
	}
	if mc.CommentID == 0 {
		_, _ = d.replyMsg(m, "❌ The replied message has no comment ID stored.")
		return nil
	}
	gqc, err := d.newGraphQLClient(ctx, m.From.ID)
	if err != nil {
		_, _ = d.replyMsg(m, "⚠️ Could not build GraphQL client. Use /replacetoken.")
		return nil
	}
	// Resolve the GraphQL Node ID of the comment via REST database ID.
	nodeID, err := gqc.LookupDiscussionCommentNodeID(ctx, mc.Owner, mc.Repo, mc.IssueNumber, mc.CommentID)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	if err := gqc.MarkDiscussionCommentAsAnswer(ctx, nodeID); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "discussion.answered", fmt.Sprintf("%s/%s#%d", mc.Owner, mc.Repo, mc.IssueNumber), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, "✅ Discussion comment marked as the answer.")
	return nil
}

// keep imports alive for future use
var (
	_ = errors.New
	_ = ghops.ErrNotFound
	_ = tgbotapi.Update{}
)
