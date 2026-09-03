// Package config loads and validates SWAGGYMUSIC GitHub Controller Bot
// configuration from environment variables (optionally backed by a .env
// file via godotenv).
//
// The Load function performs strict validation: any missing critical
// variable causes a fatal error so the bot never runs in an insecure or
// half-configured state.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config is the fully-validated runtime configuration of the bot.
type Config struct {
	// Telegram
	TelegramBotToken string
	BotOwnerIDs      []int64

	// Database
	MongoDBURI      string
	MongoDBDatabase string

	// GitHub server-level
	GitHubAPIURL           string
	GitHubToken            string // OPTIONAL owner/system PAT
	GitHubRepoURL          string // OPTIONAL owner default repo
	GitHubEnterpriseAllow  []string
	GitHubClientID         string
	GitHubClientSecret     string
	GitHubOAuthCallbackURL string
	GitHubWebhookSecret    string

	// Security
	EncryptionKey []byte
	SessionSecret []byte

	// Server
	Port          string
	PublicBaseURL string

	// Logging
	LogLevel string

	// Rate limits
	RateLimitCommandsPerMin int
	RateLimitGitHubPerMin   int
}

// Load reads environment variables and returns a validated Config.
// Missing critical variables cause a fatal error.
func Load() (*Config, error) {
	// Load .env if present. Missing file is fine (e.g. in containers).
	_ = godotenv.Load()

	cfg := &Config{
		TelegramBotToken:        strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		MongoDBURI:              strings.TrimSpace(os.Getenv("MONGODB_URI")),
		MongoDBDatabase:         strings.TrimSpace(os.Getenv("MONGODB_DATABASE")),
		GitHubAPIURL:            strings.TrimSpace(os.Getenv("GITHUB_API_URL")),
		GitHubToken:             strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		GitHubRepoURL:           strings.TrimSpace(os.Getenv("GITHUB_REPO_URL")),
		GitHubClientID:          strings.TrimSpace(os.Getenv("GITHUB_CLIENT_ID")),
		GitHubClientSecret:      strings.TrimSpace(os.Getenv("GITHUB_CLIENT_SECRET")),
		GitHubOAuthCallbackURL:  strings.TrimSpace(os.Getenv("GITHUB_OAUTH_CALLBACK_URL")),
		GitHubWebhookSecret:     strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET")),
		EncryptionKey:           decodeKey(os.Getenv("ENCRYPTION_KEY")),
		SessionSecret:           decodeKey(os.Getenv("SESSION_SECRET")),
		Port:                    strings.TrimSpace(os.Getenv("PORT")),
		PublicBaseURL:           strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")),
		LogLevel:                strings.TrimSpace(os.Getenv("LOG_LEVEL")),
		RateLimitCommandsPerMin: getEnvInt("RATE_LIMIT_COMMANDS_PER_MIN", 20),
		RateLimitGitHubPerMin:   getEnvInt("RATE_LIMIT_GITHUB_PER_MIN", 60),
	}

	// Defaults
	if cfg.MongoDBDatabase == "" {
		cfg.MongoDBDatabase = "swaggymusic_github_bot"
	}
	if cfg.GitHubAPIURL == "" {
		cfg.GitHubAPIURL = "https://api.github.com"
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.GitHubOAuthCallbackURL == "" && cfg.PublicBaseURL != "" {
		cfg.GitHubOAuthCallbackURL = cfg.PublicBaseURL + "/oauth/callback"
	}
	if cfg.RateLimitCommandsPerMin <= 0 {
		cfg.RateLimitCommandsPerMin = 20
	}
	if cfg.RateLimitGitHubPerMin <= 0 {
		cfg.RateLimitGitHubPerMin = 60
	}

	// Parse owner IDs
	for _, raw := range strings.Split(os.Getenv("BOT_OWNER_IDS"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		var id int64
		_, err := fmt.Sscanf(raw, "%d", &id)
		if err != nil || id == 0 {
			continue
		}
		cfg.BotOwnerIDs = append(cfg.BotOwnerIDs, id)
	}

	// Parse enterprise allowlist
	for _, raw := range strings.Split(os.Getenv("GITHUB_ENTERPRISE_ALLOWLIST"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		cfg.GitHubEnterpriseAllow = append(cfg.GitHubEnterpriseAllow, raw)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate verifies all critical configuration is present and well-formed.
func (c *Config) Validate() error {
	var missing []string
	if c.TelegramBotToken == "" {
		missing = append(missing, "TELEGRAM_BOT_TOKEN")
	}
	if c.MongoDBURI == "" {
		missing = append(missing, "MONGODB_URI")
	}
	if c.MongoDBDatabase == "" {
		missing = append(missing, "MONGODB_DATABASE")
	}
	if c.GitHubWebhookSecret == "" {
		missing = append(missing, "GITHUB_WEBHOOK_SECRET")
	}
	if c.PublicBaseURL == "" {
		missing = append(missing, "PUBLIC_BASE_URL")
	}
	if len(c.EncryptionKey) != 32 {
		missing = append(missing, "ENCRYPTION_KEY (must be 64 hex chars => 32 bytes)")
	}
	if len(c.SessionSecret) == 0 {
		missing = append(missing, "SESSION_SECRET")
	}
	if len(c.BotOwnerIDs) == 0 {
		missing = append(missing, "BOT_OWNER_IDS")
	}
	// OAuth is optional: if client id provided, secret+callback must be too.
	if c.GitHubClientID != "" {
		if c.GitHubClientSecret == "" {
			missing = append(missing, "GITHUB_CLIENT_SECRET (required when GITHUB_CLIENT_ID is set)")
		}
		if c.GitHubOAuthCallbackURL == "" {
			missing = append(missing, "GITHUB_OAUTH_CALLBACK_URL (required when GITHUB_CLIENT_ID is set)")
		}
	}
	if !strings.HasPrefix(c.PublicBaseURL, "https://") && !strings.HasPrefix(c.PublicBaseURL, "http://") {
		return fmt.Errorf("PUBLIC_BASE_URL must start with http:// or https:// (got %q)", c.PublicBaseURL)
	}
	// In production we strongly recommend HTTPS, but allow HTTP for local dev.
	if len(missing) > 0 {
		return fmt.Errorf("missing or invalid required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

// IsOwner returns true if the given Telegram user ID is a configured owner.
func (c *Config) IsOwner(userID int64) bool {
	for _, id := range c.BotOwnerIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// HasOAuth returns true if GitHub OAuth App credentials are configured.
func (c *Config) HasOAuth() bool {
	return c.GitHubClientID != "" && c.GitHubClientSecret != ""
}

// HasServerToken returns true if a server-level GitHub PAT is configured.
func (c *Config) HasServerToken() bool {
	return c.GitHubToken != ""
}

// decodeKey accepts a 64-char hex string and returns the 32-byte key.
// Returns nil for invalid or empty input.
func decodeKey(s string) []byte {
	s = strings.TrimSpace(s)
	if len(s) != 64 {
		return nil
	}
	out := make([]byte, 32)
	for i := 0; i < 32; i++ {
		hi := fromHexChar(s[i*2])
		lo := fromHexChar(s[i*2+1])
		if hi < 0 || lo < 0 {
			return nil
		}
		out[i] = byte(hi<<4 | lo)
	}
	return out
}

func fromHexChar(c byte) int {
	switch {
	case '0' <= c && c <= '9':
		return int(c - '0')
	case 'a' <= c && c <= 'f':
		return int(c-'a') + 10
	case 'A' <= c && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func getEnvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil {
		return fallback
	}
	return n
}
