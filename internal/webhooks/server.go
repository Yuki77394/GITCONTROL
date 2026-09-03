// Package webhooks formats GitHub webhook events into Telegram HTML
// messages and dispatches them to the right chat/topic.
package webhooks

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/swaggymusic/github-bot/internal/database"
	"github.com/swaggymusic/github-bot/internal/encryption"
	"github.com/swaggymusic/github-bot/internal/github"
	"github.com/swaggymusic/github-bot/internal/logger"
	"github.com/swaggymusic/github-bot/internal/models"
	"github.com/swaggymusic/github-bot/internal/telegram"
	"github.com/swaggymusic/github-bot/internal/webhookroutes"

	gh "github.com/google/go-github/v66/github"
)

// Server is the HTTP server receiving GitHub webhooks.
type Server struct {
	DB            *database.DB
	Bot           *telegram.Bot
	Enc           *encryption.Service
	ClientFactory *github.ClientFactory
	Log           *logger.Logger
	WebhookSecret string
	PublicBaseURL string
	Routes        *webhookroutes.Store // may be nil — falls back to encrypted-token routing
}

// New creates a Server.
func New(db *database.DB, bot *telegram.Bot, enc *encryption.Service, cf *github.ClientFactory, log *logger.Logger, secret, publicBaseURL string) *Server {
	return &Server{
		DB:            db,
		Bot:           bot,
		Enc:           enc,
		ClientFactory: cf,
		Log:           log,
		WebhookSecret: secret,
		PublicBaseURL: publicBaseURL,
	}
}

// WithRoutes attaches a webhookroutes.Store, enabling database-backed random
// route IDs. When set, the handler first tries to look up the path segment as
// a route ID; if not found, it falls back to decrypting the path as an
// encrypted token (backward compatibility with webhooks created before route
// IDs were introduced).
func (s *Server) WithRoutes(store *webhookroutes.Store) *Server {
	s.Routes = store
	return s
}

// Handler is the http.HandlerFunc that receives webhooks.
//
// URL format: /webhook/{token}  where {token} is an AES-GCM encrypted blob
// containing "<chatID>" or "<chatID>:<topicID>".
func (s *Server) Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	chatID, topicID, err := s.parseWebhookToken(r.URL.Path)
	if err != nil {
		s.Log.Warnf("webhook: invalid token: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10*1024*1024))
	if err != nil {
		s.Log.Warnf("webhook: read body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := github.VerifyWebhookSignature(r, body, s.WebhookSecret); err != nil {
		s.Log.Warnf("webhook: signature verification failed for chat %d: %v", chatID, err)
		http.Error(w, "Signature verification failed", http.StatusUnauthorized)
		return
	}

	eventType := github.EventType(r)
	deliveryID := github.DeliveryID(r)
	hookID := github.ParseHookID(r)

	if eventType == "ping" {
		s.Log.Infof("webhook: ping received (chat=%d, delivery=%s)", chatID, deliveryID)
		w.WriteHeader(http.StatusOK)
		return
	}

	payload, err := gh.ParseWebHook(gh.WebHookType(r), body)
	if err != nil {
		s.Log.Warnf("webhook: parse failed: %v", err)
		http.Error(w, "Parse error", http.StatusBadRequest)
		return
	}

	// Process asynchronously to keep webhook latency low.
	go s.processEvent(payload, chatID, topicID, hookID, eventType, deliveryID)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) parseWebhookToken(path string) (int64, int32, error) {
	const prefix = "/webhook/"
	if !strings.HasPrefix(path, prefix) || len(path) <= len(prefix) {
		return 0, 0, errors.New("missing token")
	}
	token := path[len(prefix):]

	// First, try to look up the token as a route ID (database-backed random ID).
	// This is the preferred routing mechanism — it does not expose the chat ID
	// in the URL and supports rotation/revocation.
	if s.Routes != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if route, err := s.Routes.Lookup(ctx, token); err == nil && route != nil {
			return route.ChatID, route.TopicID, nil
		}
		// If lookup failed with anything other than "not found", log it but
		// fall through to the encrypted-token path (could be a legacy webhook).
	}

	// Fallback: decrypt the token as an AES-GCM encrypted blob containing
	// "<chatID>" or "<chatID>:<topicID>". This is the legacy routing format.
	decrypted, err := s.Enc.Decrypt(token)
	if err != nil {
		return 0, 0, fmt.Errorf("decrypt: %w", err)
	}
	if strings.Contains(decrypted, ":") {
		parts := strings.SplitN(decrypted, ":", 2)
		chatID, err := parseInt64(parts[0])
		if err != nil {
			return 0, 0, err
		}
		topicID, err := parseInt32(parts[1])
		if err != nil {
			return 0, 0, err
		}
		return chatID, topicID, nil
	}
	chatID, err := parseInt64(decrypted)
	if err != nil {
		return 0, 0, err
	}
	return chatID, 0, nil
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("invalid int64: %q", s)
	}
	return n, nil
}

func parseInt32(s string) (int32, error) {
	var n int32
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("invalid int32: %q", s)
	}
	return n, nil
}

func (s *Server) processEvent(payload interface{}, chatID int64, topicID int32, hookID int64, eventType, deliveryID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Extract repo full name from the payload for per-event filtering.
	repoFullName := extractRepoFullNameFromPayload(payload)

	// Handle repo rename: update stored link name.
	if e, ok := payload.(*gh.RepositoryEvent); ok && e.GetAction() == "renamed" {
		newFullName := e.GetRepo().GetFullName()
		if newFullName != "" && hookID != 0 {
			if err := s.DB.UpdateRepoLinkName(ctx, chatID, hookID, newFullName); err != nil {
				s.Log.Warnf("webhook: update repo name: %v", err)
			}
			// Update our local var so the per-event check below uses the new name.
			repoFullName = newFullName
		}
	}

	// Check if the chat has muted this topic.
	if topicID != 0 {
		muted, _ := s.DB.IsThreadMuted(ctx, chatID, topicID)
		if muted {
			return
		}
	}

	// Per-repo, per-event notification settings.
	// If the chat has a stored RepoLink for this repo, consult its Events
	// list and Muted flag. If the repo is muted, drop the event. If the
	// event type is not in the enabled list, drop it.
	if repoFullName != "" {
		link, err := s.DB.GetRepoLink(ctx, chatID, repoFullName)
		if err == nil && link != nil {
			if link.Muted {
				s.Log.Debugf("webhook: dropping event %s for muted repo %s in chat %d", eventType, repoFullName, chatID)
				return
			}
			if len(link.Events) > 0 && !containsString(link.Events, eventType) {
				s.Log.Debugf("webhook: dropping event %s for repo %s in chat %d (not in enabled events list)", eventType, repoFullName, chatID)
				return
			}
		}
		// If no link is found, the event is from a webhook we don't track;
		// we still deliver it (legacy behaviour) to avoid silent breakage.
	}

	// Check if the repo is muted for the chat.
	msg, markup := s.formatMessage(payload, eventType)
	if msg == "" {
		return
	}

	// Send to Telegram.
	opts := &telegramSendOpts{
		ChatID:    chatID,
		Text:      msg,
		ParseMode: "HTML",
		TopicID:   topicID,
		Markup:    markup,
	}
	sentID, err := s.botSend(opts)
	if err != nil {
		s.Log.Warnf("webhook: send to chat %d: %v", chatID, err)
		return
	}

	// Store message context so replies can be forwarded back to GitHub.
	if mc := buildMessageContext(payload, chatID, sentID); mc != nil {
		if err := s.DB.SaveMessageContext(ctx, mc); err != nil {
			s.Log.Warnf("webhook: save message context: %v", err)
		}
	}
}

// extractRepoFullNameFromPayload walks the payload's repo field to find the
// "owner/repo" string. Returns "" if not found.
func extractRepoFullNameFromPayload(payload interface{}) string {
	// Most GitHub webhook events have a GetRepo() method returning a
	// *Repository with GetFullName(). We use a type switch on the common
	// event types. For events without a repo field, we return "".
	switch e := payload.(type) {
	case *gh.PushEvent:
		return e.GetRepo().GetFullName()
	case *gh.PullRequestEvent:
		return e.GetRepo().GetFullName()
	case *gh.IssuesEvent:
		return e.GetRepo().GetFullName()
	case *gh.IssueCommentEvent:
		return e.GetRepo().GetFullName()
	case *gh.PullRequestReviewEvent:
		return e.GetRepo().GetFullName()
	case *gh.PullRequestReviewCommentEvent:
		return e.GetRepo().GetFullName()
	case *gh.ReleaseEvent:
		return e.GetRepo().GetFullName()
	case *gh.ForkEvent:
		return e.GetRepo().GetFullName()
	case *gh.StarEvent:
		return e.GetRepo().GetFullName()
	case *gh.WatchEvent:
		return e.GetRepo().GetFullName()
	case *gh.WorkflowRunEvent:
		return e.GetRepo().GetFullName()
	case *gh.CheckRunEvent:
		return e.GetRepo().GetFullName()
	case *gh.CheckSuiteEvent:
		return e.GetRepo().GetFullName()
	case *gh.RepositoryEvent:
		return e.GetRepo().GetFullName()
	case *gh.CreateEvent:
		return e.GetRepo().GetFullName()
	case *gh.DeleteEvent:
		return e.GetRepo().GetFullName()
	case *gh.MemberEvent:
		return e.GetRepo().GetFullName()
	case *gh.LabelEvent:
		return e.GetRepo().GetFullName()
	case *gh.MilestoneEvent:
		return e.GetRepo().GetFullName()
	case *gh.GollumEvent:
		return e.GetRepo().GetFullName()
	case *gh.CommitCommentEvent:
		return e.GetRepo().GetFullName()
	case *gh.StatusEvent:
		return e.GetRepo().GetFullName()
	case *gh.PublicEvent:
		return e.GetRepo().GetFullName()
	case *gh.DiscussionEvent:
		return e.GetRepo().GetFullName()
	case *gh.DiscussionCommentEvent:
		return e.GetRepo().GetFullName()
	}
	return ""
}

// containsString returns true if the slice contains s.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

type telegramSendOpts struct {
	ChatID    int64
	Text      string
	ParseMode string
	TopicID   int32
	Markup    any
}

func (s *Server) botSend(o *telegramSendOpts) (int64, error) {
	return s.Bot.SendMessage(o.ChatID, o.Text, o.ParseMode, 0, o.TopicID, o.Markup)
}

func buildMessageContext(payload interface{}, chatID, messageID int64) *models.MessageContext {
	mc := &models.MessageContext{
		ChatID:    chatID,
		MessageID: messageID,
	}
	switch e := payload.(type) {
	case *gh.PullRequestEvent:
		mc.Owner = e.GetRepo().GetOwner().GetLogin()
		mc.Repo = e.GetRepo().GetName()
		mc.IssueNumber = e.GetPullRequest().GetNumber()
		mc.Type = "pr"
	case *gh.IssuesEvent:
		mc.Owner = e.GetRepo().GetOwner().GetLogin()
		mc.Repo = e.GetRepo().GetName()
		mc.IssueNumber = e.GetIssue().GetNumber()
		mc.Type = "issue"
	case *gh.IssueCommentEvent:
		mc.Owner = e.GetRepo().GetOwner().GetLogin()
		mc.Repo = e.GetRepo().GetName()
		mc.IssueNumber = e.GetIssue().GetNumber()
		mc.CommentID = e.GetComment().GetID()
		mc.Type = "issue_comment"
	case *gh.PullRequestReviewEvent:
		mc.Owner = e.GetRepo().GetOwner().GetLogin()
		mc.Repo = e.GetRepo().GetName()
		mc.IssueNumber = e.GetPullRequest().GetNumber()
		mc.Type = "pr_review"
	case *gh.PullRequestReviewCommentEvent:
		mc.Owner = e.GetRepo().GetOwner().GetLogin()
		mc.Repo = e.GetRepo().GetName()
		mc.IssueNumber = e.GetPullRequest().GetNumber()
		mc.CommentID = e.GetComment().GetID()
		mc.Type = "pr_review_comment"
	case *gh.DiscussionCommentEvent:
		// Discussion comments: IssueNumber maps to the discussion number,
		// CommentID is the REST database ID of the comment.
		mc.Owner = e.GetRepo().GetOwner().GetLogin()
		mc.Repo = e.GetRepo().GetName()
		mc.IssueNumber = e.GetDiscussion().GetNumber()
		mc.CommentID = e.GetComment().GetID()
		mc.Type = "discussion_comment"
	case *gh.DiscussionEvent:
		mc.Owner = e.GetRepo().GetOwner().GetLogin()
		mc.Repo = e.GetRepo().GetName()
		mc.IssueNumber = e.GetDiscussion().GetNumber()
		mc.Type = "discussion"
	default:
		return nil
	}
	if mc.Owner == "" || mc.Repo == "" || mc.IssueNumber == 0 {
		return nil
	}
	return mc
}

// formatMessage returns the Telegram HTML message and optional inline markup
// for a given GitHub event. Returns ("", nil) for events we don't format.
func (s *Server) formatMessage(payload interface{}, eventType string) (string, any) {
	switch e := payload.(type) {
	case *gh.PushEvent:
		return formatPush(e), nil
	case *gh.PullRequestEvent:
		return formatPullRequest(e), nil
	case *gh.IssuesEvent:
		return formatIssues(e), nil
	case *gh.IssueCommentEvent:
		return formatIssueComment(e), nil
	case *gh.PullRequestReviewEvent:
		return formatPRReview(e), nil
	case *gh.PullRequestReviewCommentEvent:
		return formatPRReviewComment(e), nil
	case *gh.ReleaseEvent:
		return formatRelease(e), nil
	case *gh.ForkEvent:
		return formatFork(e), nil
	case *gh.StarEvent:
		return formatStar(e), nil
	case *gh.WorkflowRunEvent:
		return formatWorkflowRun(e), nil
	case *gh.CheckRunEvent:
		return formatCheckRun(e), nil
	case *gh.CheckSuiteEvent:
		return formatCheckSuite(e), nil
	case *gh.RepositoryEvent:
		return formatRepository(e), nil
	case *gh.CreateEvent:
		return formatCreate(e), nil
	case *gh.DeleteEvent:
		return formatDelete(e), nil
	case *gh.MemberEvent:
		return formatMember(e), nil
	case *gh.LabelEvent:
		return formatLabel(e), nil
	case *gh.MilestoneEvent:
		return formatMilestone(e), nil
	case *gh.GollumEvent:
		return formatGollum(e), nil
	case *gh.CommitCommentEvent:
		return formatCommitComment(e), nil
	case *gh.StatusEvent:
		return formatStatus(e), nil
	case *gh.WatchEvent:
		return formatWatch(e), nil
	case *gh.PublicEvent:
		return formatPublic(e), nil
	case *gh.TeamEvent, *gh.TeamAddEvent, *gh.MembershipEvent, *gh.OrganizationEvent:
		return formatMembership(payload), nil
	case *gh.DiscussionEvent:
		return formatDiscussion(e), nil
	case *gh.DiscussionCommentEvent:
		return formatDiscussionComment(e), nil
	default:
		// Unknown event — log delivery ID for diagnostics but don't spam chats.
		s.Log.Debugf("webhook: unformatted event type %s", eventType)
		return "", nil
	}
}

// ----- Helpers -----

func esc(s string) string { return html.EscapeString(s) }

func repoLink(owner, repo string) string {
	return fmt.Sprintf(`<a href="https://github.com/%s/%s">%s/%s</a>`, esc(owner), esc(repo), esc(owner), esc(repo))
}

func userLink(login string) string {
	return fmt.Sprintf(`<a href="https://github.com/%s">@%s</a>`, esc(login), esc(login))
}

// VerifySignature is exported for tests.
func VerifySignature(r *http.Request, body []byte, secret string) error {
	return github.VerifyWebhookSignature(r, body, secret)
}

// ConstantTimeCompare is exported for tests.
func ConstantTimeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// ParseJSON is a small helper for tests.
func ParseJSON(b []byte, v any) error {
	return json.Unmarshal(b, v)
}
