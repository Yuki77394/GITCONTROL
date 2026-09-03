// Package auth implements the OAuth callback HTTP handler.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/swaggymusic/github-bot/internal/database"
	"github.com/swaggymusic/github-bot/internal/encryption"
	"github.com/swaggymusic/github-bot/internal/ghaccess"
	"github.com/swaggymusic/github-bot/internal/github"
	"github.com/swaggymusic/github-bot/internal/logger"
	"github.com/swaggymusic/github-bot/internal/models"
	"github.com/swaggymusic/github-bot/internal/telegram"

	"golang.org/x/oauth2"
)

// CallbackServer handles /oauth/callback.
type CallbackServer struct {
	DB            *database.DB
	OAuth         *github.OAuth
	Access        *ghaccess.Service
	Bot           *telegram.Bot
	Enc           *encryption.Service
	Log           *logger.Logger
	DefaultAPIURL string
}

// New creates a CallbackServer.
func New(db *database.DB, oauth *github.OAuth, access *ghaccess.Service, bot *telegram.Bot, enc *encryption.Service, log *logger.Logger, defaultAPIURL string) *CallbackServer {
	return &CallbackServer{
		DB:            db,
		OAuth:         oauth,
		Access:        access,
		Bot:           bot,
		Enc:           enc,
		Log:           log,
		DefaultAPIURL: defaultAPIURL,
	}
}

// Handler is the HTTP handler for /oauth/callback.
//
// Flow:
//  1. Receive code+state from GitHub redirect.
//  2. Atomically consume the OAuth state (single-use, expiry-checked).
//  3. Exchange code for access token.
//  4. Encrypt and store the token via ghaccess.
//  5. Notify the user in Telegram.
//  6. Render a simple success page (or error page).
func (s *CallbackServer) Handler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		renderError(w, "Missing code or state parameter.")
		return
	}

	// Consume state atomically.
	st, err := s.DB.ConsumeOAuthState(ctx, state)
	if err != nil {
		s.Log.Warnf("oauth callback: invalid or expired state: %v", err)
		renderError(w, "Invalid or expired state. Please retry /connect.")
		return
	}

	// Exchange code for token.
	tok, err := s.OAuth.ExchangeCode(ctx, code)
	if err != nil {
		s.Log.Warnf("oauth callback: exchange failed: %v", err)
		renderError(w, "Failed to exchange authorization code.")
		return
	}

	// Store via ghaccess (validates + encrypts + persists).
	username, err := s.Access.StoreOAuthToken(ctx, st.TelegramID, tok.AccessToken, s.DefaultAPIURL, nil)
	if err != nil {
		s.Log.Warnf("oauth callback: store failed: %v", err)
		renderError(w, "Failed to store GitHub account.")
		return
	}

	// Notify the user in Telegram.
	msg := fmt.Sprintf("✅ GitHub account <b>%s</b> connected successfully!", username)
	_, _ = s.Bot.SendHTML(st.TelegramID, msg)

	renderSuccess(w, username)
}

// SaveState persists an OAuth state for a Telegram user.
func (s *CallbackServer) SaveState(ctx context.Context, state string, telegramID int64) error {
	if state == "" || telegramID == 0 {
		return errors.New("auth: state and telegramID required")
	}
	now := time.Now().UTC()
	return s.DB.SaveOAuthState(ctx, &models.OAuthState{
		State:      state,
		TelegramID: telegramID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(10 * time.Minute),
	})
}

func renderSuccess(w http.ResponseWriter, username string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	body := fmt.Sprintf(`<!doctype html><html><head><title>Connected</title></head>
<body style="font-family: sans-serif; text-align: center; padding: 50px;">
<h1>✅ Authentication Successful</h1>
<p>Your GitHub account <b>%s</b> has been connected.</p>
<p>You can close this window and return to Telegram.</p>
<script>setTimeout(function(){ window.close(); }, 1500);</script>
</body></html>`, strings.ReplaceAll(username, "<", "&lt;"))
	_, _ = w.Write([]byte(body))
}

func renderError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	body := fmt.Sprintf(`<!doctype html><html><head><title>Error</title></head>
<body style="font-family: sans-serif; text-align: center; padding: 50px;">
<h1>❌ Authentication Failed</h1>
<p>%s</p>
</body></html>`, strings.ReplaceAll(msg, "<", "&lt;"))
	_, _ = w.Write([]byte(body))
}

// ScopeList parses a comma-separated scope string into a slice.
func ScopeList(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Re-export oauth2.Token so callers don't need to import oauth2 directly.
type Token = oauth2.Token
