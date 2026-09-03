// Package ghaccess manages the SWAGGYMUSIC "GitHub Access" panel and the
// lifecycle of user-connected GitHub accounts: OAuth connect, PAT add,
// token validation, encryption-at-rest, replacement, disconnection.
//
// SECURITY INVARIANTS:
//   - Plaintext tokens are NEVER stored, logged, or sent back to Telegram.
//   - After submission, only "Configured" / "Not configured" is shown.
//   - There is NO command that retrieves stored plaintext tokens.
//   - Operations on tokens are limited to: Replace, Revoke, Disconnect.
package ghaccess

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/database"
	"github.com/swaggymusic/github-bot/internal/encryption"
	"github.com/swaggymusic/github-bot/internal/github"
	"github.com/swaggymusic/github-bot/internal/models"
	"github.com/swaggymusic/github-bot/internal/validation"

	gh "github.com/google/go-github/v66/github"
)

// Service manages GitHub account connections.
type Service struct {
	DB            *database.DB
	OAuth         *github.OAuth
	Clients       *github.ClientFactory
	Enc           *encryption.Service
	Auditor       *audit.Logger
	DefaultAPIURL string
}

// New creates a Service.
func New(db *database.DB, oauth *github.OAuth, clients *github.ClientFactory, enc *encryption.Service, auditor *audit.Logger, defaultAPIURL string) *Service {
	return &Service{
		DB:            db,
		OAuth:         oauth,
		Clients:       clients,
		Enc:           enc,
		Auditor:       auditor,
		DefaultAPIURL: defaultAPIURL,
	}
}

// ConnectStatus summarises a user's GitHub access, for the access panel.
type ConnectStatus struct {
	Connected        bool
	GitHubUsername   string
	AuthMethod       string
	APIURL           string
	HasDefaultRepo   bool
	DefaultRepo      string
	HasMultipleAccts bool
	AccountCount     int
}

// GetStatus returns the user's GitHub access status.
func (s *Service) GetStatus(ctx context.Context, telegramID int64) ConnectStatus {
	st := ConnectStatus{APIURL: s.DefaultAPIURL}
	accs, err := s.DB.ListGitHubAccounts(ctx, telegramID)
	if err != nil || len(accs) == 0 {
		return st
	}
	st.Connected = true
	st.AccountCount = len(accs)
	st.HasMultipleAccts = len(accs) > 1
	// Use the default account (or first).
	var acc *models.GitHubAccount
	for i := range accs {
		if accs[i].IsDefault {
			acc = &accs[i]
			break
		}
	}
	if acc == nil {
		acc = &accs[0]
	}
	st.GitHubUsername = acc.GitHubUsername
	st.AuthMethod = acc.AuthMethod
	if acc.APIURL != "" {
		st.APIURL = acc.APIURL
	}
	return st
}

// ValidatePAT validates a Personal Access Token by calling /user.
// Returns the authenticated GitHub user, the granted scopes (from response
// headers), and an error if validation fails.
func (s *Service) ValidatePAT(ctx context.Context, token, apiURL string) (*gh.User, []string, error) {
	if token == "" {
		return nil, nil, errors.New("ghaccess: empty token")
	}
	if apiURL == "" {
		apiURL = s.DefaultAPIURL
	}
	client, err := s.Clients.NewUserClient(ctx, token, apiURL)
	if err != nil {
		return nil, nil, err
	}
	// Use a custom request so we can capture the X-OAuth-Scopes header.
	req, err := client.NewRequest("GET", "user", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("ghaccess: build /user request: %w", err)
	}
	resp, err := client.Do(ctx, req, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("ghaccess: /user call failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("ghaccess: /user returned status %d", resp.StatusCode)
	}
	scopes := parseScopesHeader(resp.Header.Get("X-OAuth-Scopes"))
	// Now fetch the user object properly.
	u, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return nil, scopes, fmt.Errorf("ghaccess: Users.Get: %w", err)
	}
	return u, scopes, nil
}

// StorePAT validates, encrypts, and stores a PAT for the user.
// Never returns the plaintext token after this call.
func (s *Service) StorePAT(ctx context.Context, telegramID int64, token, apiURL string) (string, error) {
	if apiURL == "" {
		apiURL = s.DefaultAPIURL
	}
	u, scopes, err := s.ValidatePAT(ctx, token, apiURL)
	if err != nil {
		return "", fmt.Errorf("ghaccess: PAT validation failed: %w", err)
	}
	enc, err := s.Enc.Encrypt(token)
	if err != nil {
		return "", fmt.Errorf("ghaccess: encrypt: %w", err)
	}
	acc := &models.GitHubAccount{
		TelegramID:      telegramID,
		GitHubUserID:    u.GetID(),
		GitHubUsername:  u.GetLogin(),
		GitHubAvatarURL: u.GetAvatarURL(),
		AuthMethod:      string(github.AuthMethodPAT),
		EncryptedToken:  enc,
		TokenScopes:     scopes,
		APIURL:          apiURL,
		IsDefault:       true,
		LastValidatedAt: time.Now().UTC(),
	}
	// If this is the user's first account, mark it default.
	existing, _ := s.DB.ListGitHubAccounts(ctx, telegramID)
	if len(existing) > 0 {
		acc.IsDefault = false
	}
	if err := s.DB.UpsertGitHubAccount(ctx, acc); err != nil {
		return "", fmt.Errorf("ghaccess: upsert: %w", err)
	}
	if acc.IsDefault {
		_ = s.DB.SetDefaultGitHubAccount(ctx, telegramID, u.GetID())
	}
	s.Auditor.Log(ctx, telegramID, u.GetLogin(), "ghaccess.store_pat", "self", audit.ResultSuccess, "PAT stored (encrypted)", 0)
	return u.GetLogin(), nil
}

// StoreOAuthToken stores an OAuth-exchanged token for the user.
// Called by the OAuth callback handler after code exchange.
func (s *Service) StoreOAuthToken(ctx context.Context, telegramID int64, accessToken, apiURL string, scopes []string) (string, error) {
	if accessToken == "" {
		return "", errors.New("ghaccess: empty access token")
	}
	if apiURL == "" {
		apiURL = s.DefaultAPIURL
	}
	client, err := s.Clients.NewUserClient(ctx, accessToken, apiURL)
	if err != nil {
		return "", err
	}
	u, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("ghaccess: fetch /user: %w", err)
	}
	enc, err := s.Enc.Encrypt(accessToken)
	if err != nil {
		return "", fmt.Errorf("ghaccess: encrypt: %w", err)
	}
	acc := &models.GitHubAccount{
		TelegramID:      telegramID,
		GitHubUserID:    u.GetID(),
		GitHubUsername:  u.GetLogin(),
		GitHubAvatarURL: u.GetAvatarURL(),
		AuthMethod:      string(github.AuthMethodOAuth),
		EncryptedToken:  enc,
		TokenScopes:     scopes,
		APIURL:          apiURL,
		IsDefault:       true,
		LastValidatedAt: time.Now().UTC(),
	}
	existing, _ := s.DB.ListGitHubAccounts(ctx, telegramID)
	if len(existing) > 0 {
		acc.IsDefault = false
	}
	if err := s.DB.UpsertGitHubAccount(ctx, acc); err != nil {
		return "", fmt.Errorf("ghaccess: upsert: %w", err)
	}
	if acc.IsDefault {
		_ = s.DB.SetDefaultGitHubAccount(ctx, telegramID, u.GetID())
	}
	s.Auditor.Log(ctx, telegramID, u.GetLogin(), "ghaccess.connect_oauth", "self", audit.ResultSuccess, "OAuth connected", 0)
	return u.GetLogin(), nil
}

// ReplaceToken replaces the token for an existing account.
// The old token is overwritten (and thus unrecoverable) once the new one
// validates successfully.
func (s *Service) ReplaceToken(ctx context.Context, telegramID, ghUserID int64, newToken, apiURL string) error {
	if apiURL == "" {
		apiURL = s.DefaultAPIURL
	}
	// Validate the new token first.
	u, scopes, err := s.ValidatePAT(ctx, newToken, apiURL)
	if err != nil {
		return fmt.Errorf("ghaccess: new token validation failed: %w", err)
	}
	if u.GetID() != ghUserID {
		return fmt.Errorf("ghaccess: new token belongs to GitHub user %s (ID %d), expected ID %d", u.GetLogin(), u.GetID(), ghUserID)
	}
	enc, err := s.Enc.Encrypt(newToken)
	if err != nil {
		return err
	}
	acc, err := s.DB.GetGitHubAccountByGHID(ctx, telegramID, ghUserID)
	if err != nil {
		return fmt.Errorf("ghaccess: account not found: %w", err)
	}
	acc.EncryptedToken = enc
	acc.TokenScopes = scopes
	acc.APIURL = apiURL
	acc.LastValidatedAt = time.Now().UTC()
	if err := s.DB.UpsertGitHubAccount(ctx, acc); err != nil {
		return err
	}
	s.Auditor.Log(ctx, telegramID, acc.GitHubUsername, "ghaccess.replace_token", "self", audit.ResultSuccess, "Token replaced", 0)
	return nil
}

// Disconnect removes ALL GitHub accounts for a user.
func (s *Service) Disconnect(ctx context.Context, telegramID int64) error {
	accs, _ := s.DB.ListGitHubAccounts(ctx, telegramID)
	names := make([]string, 0, len(accs))
	for _, a := range accs {
		names = append(names, a.GitHubUsername)
	}
	if err := s.DB.DeleteAllGitHubAccounts(ctx, telegramID); err != nil {
		return err
	}
	s.Auditor.Log(ctx, telegramID, strings.Join(names, ","), "ghaccess.disconnect", "self", audit.ResultSuccess, "All accounts removed", 0)
	return nil
}

// RemoveAccount removes a single GitHub account by GitHub user ID.
func (s *Service) RemoveAccount(ctx context.Context, telegramID, ghUserID int64) error {
	acc, err := s.DB.GetGitHubAccountByGHID(ctx, telegramID, ghUserID)
	if err != nil {
		return err
	}
	if err := s.DB.DeleteGitHubAccount(ctx, telegramID, ghUserID); err != nil {
		return err
	}
	s.Auditor.Log(ctx, telegramID, acc.GitHubUsername, "ghaccess.remove_account", "self", audit.ResultSuccess, "Account removed", 0)
	return nil
}

// SetDefault marks an account as default.
func (s *Service) SetDefault(ctx context.Context, telegramID, ghUserID int64) error {
	return s.DB.SetDefaultGitHubAccount(ctx, telegramID, ghUserID)
}

// GetDecryptedClient returns an authenticated GitHub client for the user's
// default account. Plaintext token stays inside this function.
func (s *Service) GetDecryptedClient(ctx context.Context, telegramID int64) (*gh.Client, *models.GitHubAccount, error) {
	acc, err := s.DB.GetGitHubAccount(ctx, telegramID)
	if err != nil {
		return nil, nil, fmt.Errorf("ghaccess: no GitHub account connected (use /connect or /addtoken)")
	}
	tok, err := s.Enc.Decrypt(acc.EncryptedToken)
	if err != nil {
		return nil, acc, fmt.Errorf("ghaccess: token decryption failed: %w", err)
	}
	c, err := s.Clients.NewUserClient(ctx, tok, acc.APIURL)
	if err != nil {
		return nil, acc, err
	}
	return c, acc, nil
}

// GetDecryptedToken returns the plaintext token for the user's default
// account, plus the account record. The caller is responsible for not
// logging the token.
//
// This is used to construct GraphQL clients (which need the raw token for
// the oauth2 transport).
func (s *Service) GetDecryptedToken(ctx context.Context, telegramID int64) (string, *models.GitHubAccount, error) {
	acc, err := s.DB.GetGitHubAccount(ctx, telegramID)
	if err != nil {
		return "", nil, fmt.Errorf("ghaccess: no GitHub account connected (use /connect or /addtoken)")
	}
	tok, err := s.Enc.Decrypt(acc.EncryptedToken)
	if err != nil {
		return "", acc, fmt.Errorf("ghaccess: token decryption failed: %w", err)
	}
	return tok, acc, nil
}

// TestConnection performs a /user GET to verify the stored token still works.
// Returns the GitHub login on success.
func (s *Service) TestConnection(ctx context.Context, telegramID int64) (string, error) {
	c, _, err := s.GetDecryptedClient(ctx, telegramID)
	if err != nil {
		return "", err
	}
	u, _, err := c.Users.Get(ctx, "")
	if err != nil {
		return "", err
	}
	return u.GetLogin(), nil
}

// ConfigureAPIURL validates and normalises a user-supplied GitHub API URL.
// The URL must be HTTPS (or http://localhost for dev) and, if it is not
// api.github.com, must match the configured enterprise allowlist.
func (s *Service) ConfigureAPIURL(raw string, enterpriseAllowlist []string) (string, error) {
	return validation.ValidateGitHubAPIURL(raw, enterpriseAllowlist)
}

func parseScopesHeader(h string) []string {
	if h == "" {
		return nil
	}
	parts := strings.Split(h, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
