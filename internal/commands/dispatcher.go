// Package commands wires all Telegram command handlers and the update
// dispatcher.
//
// The Dispatcher receives raw tgbotapi.Update events and routes them to:
//   - Command handlers (if the message starts with /command)
//   - Reply handler (if the message is a reply to a known notification)
//   - Callback handler (if it's a callback query)
//
// All handlers run in their own goroutine so a slow handler never blocks
// another. Errors are logged but never propagated to the dispatcher (which
// would otherwise stop the update loop).
package commands

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/cache"
	"github.com/swaggymusic/github-bot/internal/config"
	"github.com/swaggymusic/github-bot/internal/database"
	"github.com/swaggymusic/github-bot/internal/encryption"
	"github.com/swaggymusic/github-bot/internal/ghaccess"
	"github.com/swaggymusic/github-bot/internal/ghops"
	"github.com/swaggymusic/github-bot/internal/github"
	"github.com/swaggymusic/github-bot/internal/graphqlclient"
	"github.com/swaggymusic/github-bot/internal/logger"
	"github.com/swaggymusic/github-bot/internal/models"
	"github.com/swaggymusic/github-bot/internal/permissions"
	"github.com/swaggymusic/github-bot/internal/ratelimit"
	"github.com/swaggymusic/github-bot/internal/replyctx"
	"github.com/swaggymusic/github-bot/internal/telegram"
	"github.com/swaggymusic/github-bot/internal/validation"
	"github.com/swaggymusic/github-bot/internal/webhookroutes"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Deps holds all shared dependencies for command handlers.
type Deps struct {
	Cfg         *config.Config
	DB          *database.DB
	Bot         *telegram.Bot
	OAuth       *github.OAuth
	Access      *ghaccess.Service
	Clients     *github.ClientFactory
	Enc         *encryption.Service
	Perms       *permissions.Service
	Reply       *replyctx.Handler
	Audit       *audit.Logger
	Log         *logger.Logger
	CmdLimiter  *ratelimit.Limiter
	GHLimiter   *ratelimit.Limiter
	StateCache  *cache.Cache[string, int64]
	ActionCache *cache.Cache[string, PRActionContext]
	OAuthSaver  OAuthStateSaver
	Routes      *webhookroutes.Store // may be nil — falls back to encrypted-token webhook URLs
}

// OAuthStateSaver persists OAuth states to DB (so they survive restarts).
type OAuthStateSaver interface {
	SaveState(ctx context.Context, state string, telegramID int64) error
}

// PRActionContext stores PR action metadata for callback confirmation flows.
type PRActionContext struct {
	Owner    string
	Repo     string
	PRNumber int
	Method   string // merge method for /merge confirmation
}

// Dispatcher routes Telegram updates.
type Dispatcher struct {
	deps     *Deps
	handlers map[string]Handler
	mu       sync.RWMutex
}

// Handler is a single command handler.
type Handler struct {
	Run         func(ctx context.Context, m *tgbotapi.Message, args []string) error
	HelpText    string
	AdminOnly   bool // requires Telegram chat admin or owner
	OwnerOnly   bool // requires bot owner
	PrivateOnly bool // only allowed in 1:1 private chats
}

// NewDispatcher creates a Dispatcher with all commands registered.
func NewDispatcher(deps *Deps) *Dispatcher {
	d := &Dispatcher{
		deps:     deps,
		handlers: make(map[string]Handler),
	}
	d.registerAll()
	return d
}

// Handle routes a single update.
func (d *Dispatcher) Handle(ctx context.Context, update tgbotapi.Update) {
	if update.Message != nil {
		d.handleMessage(ctx, update.Message)
		return
	}
	if update.CallbackQuery != nil {
		d.handleCallback(ctx, update.CallbackQuery)
		return
	}
}

func (d *Dispatcher) handleMessage(ctx context.Context, m *tgbotapi.Message) {
	// Track chat in DB (async).
	go d.trackChat(context.Background(), m)

	// Track user (async).
	go d.trackUser(context.Background(), m)

	if m.IsCommand() {
		d.handleCommand(ctx, m)
		return
	}

	// Reply to a notification → forward as GitHub comment.
	if m.ReplyToMessage != nil && m.Text != "" {
		// Don't forward if the reply text itself is a command.
		if err := d.deps.Reply.HandleReply(ctx, m.Chat.ID, int64(m.ReplyToMessage.MessageID), m.From.ID, m.Text); err != nil {
			d.deps.Log.Warnf("reply handler: %v", err)
		}
	}
}

func (d *Dispatcher) handleCommand(ctx context.Context, m *tgbotapi.Message) {
	cmd := m.Command()
	args := strings.Fields(m.CommandArguments())
	// Also keep the raw argument string for commands that take free text.
	_ = m.CommandArguments()

	d.mu.RLock()
	h, ok := d.handlers[cmd]
	d.mu.RUnlock()
	if !ok {
		// Unknown command — ignore silently (avoid spam).
		return
	}

	// Rate limit per user.
	if !d.deps.CmdLimiter.Allow(m.From.ID) {
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, "⚠️ Too many commands. Please slow down.")
		return
	}

	// Private-only check.
	if h.PrivateOnly && !telegram.IsPrivate(m.Chat.Type) {
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, "⚠️ This command can only be used in a private chat with the bot.")
		return
	}

	// Owner-only check.
	if h.OwnerOnly && !d.deps.Perms.IsOwner(m.From.ID) {
		d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "command."+cmd, "denied", audit.ResultDenied, "owner-only", m.Chat.ID)
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, "⛔ This command is restricted to bot owners.")
		return
	}

	// Admin-only check (for non-private chats).
	if h.AdminOnly && !telegram.IsPrivate(m.Chat.Type) {
		if !d.deps.Perms.IsChatAdmin(ctx, m.Chat.ID, m.From.ID) {
			d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "command."+cmd, "denied", audit.ResultDenied, "admin-only", m.Chat.ID)
			_, _ = d.deps.Bot.SendHTML(m.Chat.ID, "⛔ Only chat admins can use this command here.")
			return
		}
	}

	// Run with timeout.
	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := h.Run(cmdCtx, m, args); err != nil {
		d.deps.Log.Warnf("command /%s by %d: %v", cmd, m.From.ID, err)
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, fmt.Sprintf("⚠️ Error: %v", err))
	}
}

func (d *Dispatcher) handleCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) {
	data := cq.Data
	if data == "" {
		// Answer to clear the loading spinner even for empty data.
		_ = d.deps.Bot.AnswerCallback(cq.ID, "", false)
		return
	}
	// Route by prefix. Each handler is responsible for answering the
	// callback (via d.deps.Bot.AnswerCallback) so it can include a
	// contextual toast message. If a handler forgets to answer, Telegram
	// will keep the spinner spinning for ~10 seconds then auto-clear.
	switch {
	case strings.HasPrefix(data, "c:"):
		d.handleSettingsCallback(ctx, cq, data)
	case strings.HasPrefix(data, "act:"):
		d.handleActionCallback(ctx, cq, data)
	case strings.HasPrefix(data, "gh:"):
		d.handleAccessCallback(ctx, cq, data)
	default:
		// Unknown prefix — answer to clear the spinner.
		_ = d.deps.Bot.AnswerCallback(cq.ID, "Unknown action", false)
	}
}

// trackChat upserts the chat into the DB.
func (d *Dispatcher) trackChat(ctx context.Context, m *tgbotapi.Message) {
	if m.Chat == nil {
		return
	}
	c := &models.Chat{
		ID:       m.Chat.ID,
		ChatType: m.Chat.Type,
		Title:    m.Chat.Title,
		Username: m.Chat.UserName,
	}
	if err := d.deps.DB.UpsertChat(ctx, c); err != nil {
		d.deps.Log.Debugf("track chat: %v", err)
	}
}

// trackUser upserts the user into the DB.
func (d *Dispatcher) trackUser(ctx context.Context, m *tgbotapi.Message) {
	if m.From == nil {
		return
	}
	u := &models.User{
		TelegramID: m.From.ID,
		Username:   m.From.UserName,
		FirstName:  m.From.FirstName,
		LastName:   m.From.LastName,
		IsOwner:    d.deps.Perms.IsOwner(m.From.ID),
		LastSeenAt: time.Now().UTC(),
	}
	if err := d.deps.DB.UpsertUser(ctx, u); err != nil {
		d.deps.Log.Debugf("track user: %v", err)
	}
}

// Register a command (override existing).
func (d *Dispatcher) Register(name string, h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[name] = h
}

// registerAll registers every command the bot supports.
func (d *Dispatcher) registerAll() {
	d.registerAuth()
	d.registerRepo()
	d.registerIssue()
	d.registerPR()
	d.registerActions()
	d.registerReleases()
	d.registerCommits()
	d.registerSearch()
	d.registerBranches()
	d.registerFiles()
	d.registerDiscussions()
	d.registerSettings()
	d.registerMisc()
}

// ---------------------------------------------------------------------------
// Placeholder callback handlers — implemented in callbacks.go
// ---------------------------------------------------------------------------

func (d *Dispatcher) handleSettingsCallback(ctx context.Context, cq *tgbotapi.CallbackQuery, data string) {
	d.dispatchSettingsCallback(ctx, cq, data)
}

func (d *Dispatcher) handleActionCallback(ctx context.Context, cq *tgbotapi.CallbackQuery, data string) {
	d.dispatchActionCallback(ctx, cq, data)
}

func (d *Dispatcher) handleAccessCallback(ctx context.Context, cq *tgbotapi.CallbackQuery, data string) {
	d.dispatchAccessCallback(ctx, cq, data)
}

// PRActionContextKey generates a unique cache key for a callback message.
func PRActionContextKey(chatID, messageID int64) string {
	return fmt.Sprintf("%d:%d", chatID, messageID)
}

// SetPRActionContext stores PR action metadata for later callback confirmation.
func (d *Dispatcher) SetPRActionContext(key string, ctx PRActionContext) {
	d.deps.ActionCache.Set(key, ctx, 10*time.Minute)
}

// GetPRActionContext retrieves stored PR action metadata.
func (d *Dispatcher) GetPRActionContext(key string) (PRActionContext, bool) {
	return d.deps.ActionCache.Get(key)
}

// newGraphQLClient builds a graphqlclient.Client authenticated as the user's
// default GitHub account. Returns nil (no error) if the user has no
// connected account — callers should handle nil as "unsupported".
//
// The plaintext token never leaves this method; it is passed directly into
// graphqlclient.NewClient which stores it only inside the oauth2 transport.
func (d *Dispatcher) newGraphQLClient(ctx context.Context, telegramID int64) (*graphqlclient.Client, error) {
	token, acc, err := d.deps.Access.GetDecryptedToken(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	apiURL := d.deps.Cfg.GitHubAPIURL
	if acc != nil && acc.APIURL != "" {
		apiURL = acc.APIURL
	}
	return graphqlclient.NewClient(ctx, token, apiURL)
}

// helper imports kept here so the file compiles cleanly even if a sub-file
// forgets one of them.
var (
	_ = ghops.ErrNotFound
	_ = validation.ValidateRepoName
)
