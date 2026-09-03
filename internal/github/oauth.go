package github

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"

	"github.com/swaggymusic/github-bot/internal/config"

	"golang.org/x/oauth2"
	oauth2gh "golang.org/x/oauth2/github"
)

// OAuth wraps the GitHub OAuth App configuration.
type OAuth struct {
	Config *config.Config
	OAConf *oauth2.Config
}

// NewOAuth builds an OAuth helper. Returns nil if OAuth is not configured.
func NewOAuth(cfg *config.Config) *OAuth {
	if !cfg.HasOAuth() {
		return nil
	}
	redirectURL := cfg.GitHubOAuthCallbackURL
	if redirectURL == "" {
		redirectURL = cfg.PublicBaseURL + "/oauth/callback"
	}
	return &OAuth{
		Config: cfg,
		OAConf: &oauth2.Config{
			ClientID:     cfg.GitHubClientID,
			ClientSecret: cfg.GitHubClientSecret,
			Endpoint:     oauth2gh.Endpoint,
			Scopes:       []string{"repo", "admin:repo_hook", "read:user", "user:email"},
			RedirectURL:  redirectURL,
		},
	}
}

// GetLoginURL returns the OAuth authorization URL for the given state.
func (o *OAuth) GetLoginURL(state string) string {
	return o.OAConf.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// ExchangeCode exchanges an authorization code for an OAuth token.
func (o *OAuth) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	if code == "" {
		return nil, errors.New("oauth: empty code")
	}
	return o.OAConf.Exchange(ctx, code)
}

// GenerateState returns a cryptographically random 32-char hex string.
// Suitable for OAuth `state` parameter (CSRF protection).
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauth: rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ValidateRedirectURL ensures the configured OAuth callback URL is HTTPS
// (or localhost for dev) and matches the expected path.
func ValidateRedirectURL(raw string) error {
	if raw == "" {
		return errors.New("oauth: callback URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("oauth: callback URL parse: %w", err)
	}
	if u.Scheme != "https" && u.Hostname() != "localhost" {
		return errors.New("oauth: callback URL must be HTTPS (or http://localhost for dev)")
	}
	if u.Path != "/oauth/callback" {
		return errors.New("oauth: callback URL path must be /oauth/callback")
	}
	return nil
}

// ConstantTimeEqual compares two strings in constant time. Exposed for tests.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
