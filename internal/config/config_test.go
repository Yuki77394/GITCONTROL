package config

import (
	"os"
	"testing"
)

func setEnv(t *testing.T, kvs map[string]string) {
	t.Helper()
	os.Clearenv()
	for k, v := range kvs {
		_ = os.Setenv(k, v)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	setEnv(t, nil)
	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for missing required vars")
	}
}

func TestLoadValid(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_BOT_TOKEN":    "123:ABC",
		"BOT_OWNER_IDS":         "111,222",
		"MONGODB_URI":           "mongodb://localhost:27017",
		"MONGODB_DATABASE":      "swaggymusic_test",
		"GITHUB_WEBHOOK_SECRET": "abcdef",
		"ENCRYPTION_KEY":        "a1b2c3d4e5f60718293a4b5c6d7e8f900102030405060708090a0b0c0d0e0f10",
		"SESSION_SECRET":        "b1b2c3d4e5f60718293a4b5c6d7e8f900102030405060708090a0b0c0d0e0f10",
		"PUBLIC_BASE_URL":       "https://example.com",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.BotOwnerIDs) != 2 {
		t.Errorf("owners: got %d, want 2", len(cfg.BotOwnerIDs))
	}
	if cfg.BotOwnerIDs[0] != 111 || cfg.BotOwnerIDs[1] != 222 {
		t.Errorf("owners: got %v, want [111, 222]", cfg.BotOwnerIDs)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Errorf("encryption key length: got %d, want 32", len(cfg.EncryptionKey))
	}
	if cfg.MongoDBDatabase != "swaggymusic_test" {
		t.Errorf("db: got %q, want swaggymusic_test", cfg.MongoDBDatabase)
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_BOT_TOKEN":    "123:ABC",
		"BOT_OWNER_IDS":         "111",
		"MONGODB_URI":           "mongodb://localhost:27017",
		"GITHUB_WEBHOOK_SECRET": "abcdef",
		"ENCRYPTION_KEY":        "a1b2c3d4e5f60718293a4b5c6d7e8f900102030405060708090a0b0c0d0e0f10",
		"SESSION_SECRET":        "b1b2c3d4e5f60718293a4b5c6d7e8f900102030405060708090a0b0c0d0e0f10",
		"PUBLIC_BASE_URL":       "https://example.com",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MongoDBDatabase != "swaggymusic_github_bot" {
		t.Errorf("default db: got %q, want swaggymusic_github_bot", cfg.MongoDBDatabase)
	}
	if cfg.GitHubAPIURL != "https://api.github.com" {
		t.Errorf("default api url: got %q", cfg.GitHubAPIURL)
	}
	if cfg.Port != "8080" {
		t.Errorf("default port: got %q", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default log level: got %q", cfg.LogLevel)
	}
}

func TestIsOwner(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_BOT_TOKEN":    "123:ABC",
		"BOT_OWNER_IDS":         "111,222,333",
		"MONGODB_URI":           "mongodb://localhost:27017",
		"GITHUB_WEBHOOK_SECRET": "abcdef",
		"ENCRYPTION_KEY":        "a1b2c3d4e5f60718293a4b5c6d7e8f900102030405060708090a0b0c0d0e0f10",
		"SESSION_SECRET":        "b1b2c3d4e5f60718293a4b5c6d7e8f900102030405060708090a0b0c0d0e0f10",
		"PUBLIC_BASE_URL":       "https://example.com",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.IsOwner(111) {
		t.Errorf("IsOwner(111) = false, want true")
	}
	if !cfg.IsOwner(333) {
		t.Errorf("IsOwner(333) = false, want true")
	}
	if cfg.IsOwner(999) {
		t.Errorf("IsOwner(999) = true, want false")
	}
}

func TestInvalidEncryptionKey(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_BOT_TOKEN":    "123:ABC",
		"BOT_OWNER_IDS":         "111",
		"MONGODB_URI":           "mongodb://localhost:27017",
		"GITHUB_WEBHOOK_SECRET": "abcdef",
		"ENCRYPTION_KEY":        "too-short",
		"SESSION_SECRET":        "ok-secret",
		"PUBLIC_BASE_URL":       "https://example.com",
	})
	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for short encryption key")
	}
}

func TestInvalidPublicBaseURL(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_BOT_TOKEN":    "123:ABC",
		"BOT_OWNER_IDS":         "111",
		"MONGODB_URI":           "mongodb://localhost:27017",
		"GITHUB_WEBHOOK_SECRET": "abcdef",
		"ENCRYPTION_KEY":        "a1b2c3d4e5f60718293a4b5c6d7e8f900102030405060708090a0b0c0d0e0f10",
		"SESSION_SECRET":        "b1b2c3d4e5f60718293a4b5c6d7e8f900102030405060708090a0b0c0d0e0f10",
		"PUBLIC_BASE_URL":       "ftp://example.com",
	})
	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for non-http(s) public base URL")
	}
}
