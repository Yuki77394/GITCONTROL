// Package permissions provides role-based access control for the bot.
//
// The bot separates Telegram-level permissions (owner, chat admin) from
// GitHub-level permissions (validated independently per request via the
// GitHub API). Telegram admin status NEVER automatically grants GitHub
// repository access.
package permissions

import (
	"context"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/swaggymusic/github-bot/internal/cache"
	"github.com/swaggymusic/github-bot/internal/config"
)

// Role enumerates Telegram-side roles.
type Role int

const (
	RoleUnknown Role = iota
	RoleNormal
	RoleChatAdmin
	RoleOwner
)

// String returns a human label.
func (r Role) String() string {
	switch r {
	case RoleNormal:
		return "normal"
	case RoleChatAdmin:
		return "chat_admin"
	case RoleOwner:
		return "owner"
	default:
		return "unknown"
	}
}

// Service provides role lookups with caching.
type Service struct {
	cfg     *config.Config
	bot     *tgbotapi.BotAPI
	adminMu sync.Mutex
	admins  *cache.Cache[int64, []int64]
}

// New creates a permissions Service.
func New(cfg *config.Config, bot *tgbotapi.BotAPI) *Service {
	return &Service{
		cfg:    cfg,
		bot:    bot,
		admins: cache.New[int64, []int64](),
	}
}

// IsOwner returns true if the user is a configured bot owner.
func (s *Service) IsOwner(userID int64) bool {
	if s == nil || s.cfg == nil {
		return false
	}
	return s.cfg.IsOwner(userID)
}

// IsChatAdmin returns true if the user is an administrator of the given chat.
// Results are cached for 1 hour. Owner is always considered an admin.
func (s *Service) IsChatAdmin(ctx context.Context, chatID, userID int64) bool {
	if s.IsOwner(userID) {
		return true
	}
	if cached, ok := s.admins.Get(chatID); ok {
		return contains(cached, userID)
	}
	// Fetch admins via Telegram API.
	members, err := s.bot.GetChatAdministrators(tgbotapi.ChatAdministratorsConfig{
		ChatConfig: tgbotapi.ChatConfig{ChatID: chatID},
	})
	if err != nil {
		return false
	}
	ids := make([]int64, 0, len(members))
	isAdmin := false
	for _, m := range members {
		ids = append(ids, m.User.ID)
		if m.User.ID == userID {
			isAdmin = true
		}
	}
	s.admins.Set(chatID, ids, 1*time.Hour)
	return isAdmin
}

// RoleOf returns the role for a user in a given chat context.
func (s *Service) RoleOf(ctx context.Context, chatID, userID int64) Role {
	if s.IsOwner(userID) {
		return RoleOwner
	}
	if s.IsChatAdmin(ctx, chatID, userID) {
		return RoleChatAdmin
	}
	return RoleNormal
}

// InvalidateAdminCache clears the cached admin list for a chat (e.g. on /reload).
func (s *Service) InvalidateAdminCache(chatID int64) {
	if s == nil {
		return
	}
	s.admins.Delete(chatID)
}

// InvalidateAllAdminCache clears the entire admin cache.
func (s *Service) InvalidateAllAdminCache() {
	if s == nil {
		return
	}
	// Re-create the cache (cheap; just clears entries).
	s.adminMu.Lock()
	defer s.adminMu.Unlock()
	s.admins = cache.New[int64, []int64]()
}

func contains(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// SplitArgs parses a raw argument string into trimmed tokens, respecting
// double-quoted substrings. Helper for command handlers.
func SplitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t' || r == '\n') && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
