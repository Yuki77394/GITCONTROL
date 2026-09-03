// Package replyctx implements the "reply to a Telegram notification to
// post a GitHub comment" feature.
//
// When a webhook notification is sent to a chat, the bot stores a
// MessageContext mapping (chat_id, message_id) -> GitHub (owner, repo,
// issue_number, comment_id, type). When a user replies to that message
// in Telegram with non-command text, the bot uses the mapping to post
// the reply as a GitHub comment.
//
// SECURITY:
//   - Only replies to messages with a stored mapping are forwarded.
//   - The replier must have a connected GitHub account.
//   - GitHub permission is validated by the GitHub API itself (403 otherwise).
//   - The mapping expires after 48 hours.
package replyctx

import (
	"context"
	"fmt"

	"github.com/swaggymusic/github-bot/internal/database"
	"github.com/swaggymusic/github-bot/internal/encryption"
	"github.com/swaggymusic/github-bot/internal/github"
	"github.com/swaggymusic/github-bot/internal/logger"
	"github.com/swaggymusic/github-bot/internal/telegram"

	gh "github.com/google/go-github/v66/github"
)

// Handler handles reply-to-GitHub forwarding.
type Handler struct {
	DB            *database.DB
	Enc           *encryption.Service
	ClientFactory *github.ClientFactory
	Bot           *telegram.Bot
	Log           *logger.Logger
}

// New creates a Handler.
func New(db *database.DB, enc *encryption.Service, cf *github.ClientFactory, bot *telegram.Bot, log *logger.Logger) *Handler {
	return &Handler{
		DB:            db,
		Enc:           enc,
		ClientFactory: cf,
		Bot:           bot,
		Log:           log,
	}
}

// HandleReply processes a reply in Telegram. If the replied-to message has
// a stored GitHub context, the reply text is posted as a GitHub comment.
// Returns nil silently if there is no mapping (so non-GitHub replies are
// ignored).
func (h *Handler) HandleReply(ctx context.Context, chatID, replyToMessageID, senderID int64, text string) error {
	mc, err := h.DB.GetMessageContext(ctx, chatID, replyToMessageID)
	if err != nil {
		// No mapping found — silently ignore.
		return nil
	}
	if text == "" {
		return nil
	}

	// Look up the replier's GitHub account.
	acc, err := h.DB.GetGitHubAccount(ctx, senderID)
	if err != nil {
		_, _ = h.Bot.SendHTML(chatID, "⚠️ You have not connected a GitHub account. Use /connect first.")
		return nil
	}
	token, err := h.Enc.Decrypt(acc.EncryptedToken)
	if err != nil {
		_, _ = h.Bot.SendHTML(chatID, "⚠️ Could not decrypt your token. Please reconnect via /connect.")
		return nil
	}
	client, err := h.ClientFactory.NewUserClient(ctx, token, acc.APIURL)
	if err != nil {
		h.Log.Warnf("replyctx: client: %v", err)
		_, _ = h.Bot.SendHTML(chatID, "⚠️ Failed to create GitHub client.")
		return nil
	}

	// For PR review comments, post as a reply to the original review comment.
	if mc.Type == "pr_review_comment" && mc.CommentID != 0 {
		comment := &gh.PullRequestComment{
			Body:      gh.String(text),
			InReplyTo: gh.Int64(mc.CommentID),
		}
		_, _, err = client.PullRequests.CreateComment(ctx, mc.Owner, mc.Repo, mc.IssueNumber, comment)
	} else {
		_, _, err = client.Issues.CreateComment(ctx, mc.Owner, mc.Repo, mc.IssueNumber, &gh.IssueComment{Body: gh.String(text)})
	}
	if err != nil {
		h.Log.Warnf("replyctx: post comment to %s/%s#%d: %v", mc.Owner, mc.Repo, mc.IssueNumber, err)
		_, _ = h.Bot.SendHTML(chatID, fmt.Sprintf("⚠️ Failed to post comment: %v", err))
		return nil
	}
	_, _ = h.Bot.SendHTML(chatID, "✅ Comment posted to GitHub.")
	return nil
}
