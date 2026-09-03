// Package main is the entry point for the SWAGGYMUSIC GitHub Controller Bot.
//
// The bot is a long-running process that:
//  1. Loads configuration from environment (optionally from .env).
//  2. Connects to MongoDB.
//  3. Initializes the Telegram bot (long polling — no separate webhook
//     server needed for Telegram updates).
//  4. Starts an HTTP server for GitHub OAuth callback and GitHub webhooks.
//  5. Routes Telegram updates through the command dispatcher.
//
// Graceful shutdown on SIGINT/SIGTERM: in-flight HTTP requests are allowed
// up to 10 seconds to finish, the Telegram update loop is stopped, and the
// MongoDB client is disconnected.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/auth"
	"github.com/swaggymusic/github-bot/internal/cache"
	"github.com/swaggymusic/github-bot/internal/commands"
	"github.com/swaggymusic/github-bot/internal/config"
	"github.com/swaggymusic/github-bot/internal/database"
	"github.com/swaggymusic/github-bot/internal/encryption"
	"github.com/swaggymusic/github-bot/internal/ghaccess"
	"github.com/swaggymusic/github-bot/internal/github"
	"github.com/swaggymusic/github-bot/internal/logger"
	"github.com/swaggymusic/github-bot/internal/permissions"
	"github.com/swaggymusic/github-bot/internal/ratelimit"
	"github.com/swaggymusic/github-bot/internal/replyctx"
	"github.com/swaggymusic/github-bot/internal/telegram"
	"github.com/swaggymusic/github-bot/internal/webhookroutes"
	"github.com/swaggymusic/github-bot/internal/webhooks"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Load config.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.Default(logger.ParseLevel(cfg.LogLevel))
	log.Infof("SWAGGYMUSIC GitHub Controller Bot starting up")
	log.Infof("Public base URL: %s", cfg.PublicBaseURL)
	log.Infof("GitHub API URL: %s", cfg.GitHubAPIURL)
	log.Infof("Bot owners: %d configured", len(cfg.BotOwnerIDs))
	if cfg.HasOAuth() {
		log.Infof("GitHub OAuth: configured (callback=%s)", cfg.GitHubOAuthCallbackURL)
	} else {
		log.Infof("GitHub OAuth: NOT configured (PAT-only mode)")
	}
	if cfg.HasServerToken() {
		log.Infof("Server-level GitHub token: configured (currently unused by any command — reserved for future owner-only operations)")
	}

	// 2. Connect MongoDB.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Connect(ctx, cfg.MongoDBURI, cfg.MongoDBDatabase)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = db.Disconnect(shutdownCtx)
		log.Infof("MongoDB disconnected")
	}()
	log.Infof("MongoDB connected: %s", redactMongoURI(cfg.MongoDBURI))

	// 3. Initialize encryption service.
	enc, err := encryption.New(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("encryption: %w", err)
	}
	log.Infof("Encryption service initialised (AES-256-GCM)")

	// 4. Initialize Telegram bot.
	bot, err := telegram.New(cfg.TelegramBotToken, cfg.LogLevel == "debug")
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	log.Infof("Telegram bot started: @%s", bot.Username())

	// 5. Initialize services.
	oauth := github.NewOAuth(cfg)
	clients := github.NewClientFactory(cfg.GitHubAPIURL)
	auditor := audit.New(db)
	access := ghaccess.New(db, oauth, clients, enc, auditor, cfg.GitHubAPIURL)
	perms := permissions.New(cfg, bot.API())
	cmdLimiter := ratelimit.New(cfg.RateLimitCommandsPerMin)
	ghLimiter := ratelimit.New(cfg.RateLimitGitHubPerMin)
	stateCache := cache.New[string, int64]()
	actionCache := cache.New[string, commands.PRActionContext]()
	reply := replyctx.New(db, enc, clients, bot, log)

	// OAuth callback server (also persists states).
	cbServer := auth.New(db, oauth, access, bot, enc, log, cfg.GitHubAPIURL)

	// Webhook server.
	whServer := webhooks.New(db, bot, enc, clients, log, cfg.GitHubWebhookSecret, cfg.PublicBaseURL)

	// Webhook route ID store (random opaque IDs backed by MongoDB).
	routes, err := webhookroutes.New(db.Database)
	if err != nil {
		log.Warnf("webhookroutes: could not init store (falling back to encrypted-token routing): %v", err)
	} else {
		whServer.WithRoutes(routes)
		log.Infof("Webhook route ID store initialised (new webhooks use opaque route IDs)")
	}

	// 6. Build the dispatcher.
	deps := &commands.Deps{
		Cfg:         cfg,
		DB:          db,
		Bot:         bot,
		OAuth:       oauth,
		Access:      access,
		Clients:     clients,
		Enc:         enc,
		Perms:       perms,
		Reply:       reply,
		Audit:       auditor,
		Log:         log,
		CmdLimiter:  cmdLimiter,
		GHLimiter:   ghLimiter,
		StateCache:  stateCache,
		ActionCache: actionCache,
		OAuthSaver:  cbServer,
		Routes:      routes,
	}
	dispatcher := commands.NewDispatcher(deps)

	// 7. Start HTTP server for OAuth callback + webhooks + health.
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", cbServer.Handler)
	mux.HandleFunc("/webhook/", whServer.Handler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Lightweight health endpoint — no auth, no secrets, no GitHub calls.
		// Used by Heroku health checks, Docker HEALTHCHECK, and load balancers.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// We deliberately do NOT include MongoDB ping here to keep the
		// endpoint cheap and avoid amplifying DB load under health-check
		// polling. The DB connection was validated at startup; if it drops
		// later, webhook/command handlers will surface errors.
		resp := map[string]string{
			"status":  "ok",
			"service": "swaggymusic-github-bot",
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>SWAGGYMUSIC GitHub Bot</title></head>
<body style="font-family: sans-serif; text-align: center; padding: 50px;">
<h1>SWAGGYMUSIC GitHub Controller Bot</h1>
<p>The bot is running successfully.</p>
</body></html>`))
	})
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 8. Start HTTP server in a goroutine.
	go func() {
		log.Infof("HTTP server listening on port %s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("http server: %v", err)
		}
	}()

	// 9. Start Telegram long-polling in a goroutine.
	updateCtx, updateCancel := context.WithCancel(context.Background())
	defer updateCancel()
	updates := bot.GetUpdatesChan(updateCtx)
	go func() {
		for update := range updates {
			// Each update gets its own context with a timeout.
			updCtx, c := context.WithTimeout(context.Background(), 60*time.Second)
			dispatcher.Handle(updCtx, update)
			c()
		}
	}()

	log.Infof("Bot is ready. Send /start to @%s in Telegram.", bot.Username())

	// 10. Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Infof("Received signal %v, shutting down...", sig)

	// Stop Telegram updates.
	bot.Stop()
	updateCancel()

	// Shutdown HTTP server.
	shutdownCtx, sCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer sCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Errorf("http shutdown: %v", err)
	}

	log.Infof("Shutdown complete. Bye!")
	return nil
}

// redactMongoURI returns a copy of the MongoDB connection string with the
// password (if present) replaced by "***". This allows logging the URI for
// diagnostics without leaking credentials.
//
// Handles both standard and SRV URI formats:
//
//	mongodb://user:password@host:port/db  →  mongodb://user:***@host:port/db
//	mongodb+srv://user:password@host/db   →  mongodb+srv://user:***@host/db
//
// If parsing fails for any reason, returns "mongodb://***" (safe fallback).
func redactMongoURI(uri string) string {
	if uri == "" {
		return "(empty)"
	}
	// Find the scheme separator.
	schemeEnd := strings.Index(uri, "://")
	if schemeEnd < 0 {
		return "(invalid-uri)"
	}
	scheme := uri[:schemeEnd+3]
	rest := uri[schemeEnd+3:]
	// Find the credentials separator (before @).
	atIdx := strings.Index(rest, "@")
	if atIdx < 0 {
		// No credentials in URI — safe to log as-is.
		return scheme + rest
	}
	creds := rest[:atIdx]
	hostAndPath := rest[atIdx:]
	// Redact the password part (after the colon).
	colonIdx := strings.Index(creds, ":")
	if colonIdx < 0 {
		// No password, just username — still redact to be safe.
		return scheme + "***" + hostAndPath
	}
	username := creds[:colonIdx]
	return scheme + username + ":***" + hostAndPath
}
