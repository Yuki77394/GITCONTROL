# SWAGGYMUSIC GitHub Controller Bot — Heroku Deployment Guide

This document is a complete checklist for deploying the SWAGGYMUSIC GitHub
Controller Bot to Heroku using the container stack (`heroku.yml` + existing
`Dockerfile`).

> **No MongoDB runs inside the Heroku dyno.** MongoDB must be hosted
> externally (e.g. MongoDB Atlas). The bot connects via `MONGODB_URI`.

---

## Architecture on Heroku

```
┌──────────────────────────────────────────────────────────────┐
│  Heroku Dyno (web process, container stack)                  │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  swaggymusic-bot (Go binary, static, non-root)         │  │
│  │                                                        │  │
│  │  • HTTP server on $PORT (0.0.0.0)                      │  │
│  │    - /health        (health check)                     │  │
│  │    - /oauth/callback (GitHub OAuth)                    │  │
│  │    - /webhook/{id}  (GitHub webhooks)                  │  │
│  │  • Telegram long-polling                               │  │
│  │  • Graceful shutdown on SIGTERM                        │  │
│  └────────────────────────────────────────────────────────┘  │
│                         │                                    │
└─────────────────────────┼────────────────────────────────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
   ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
   │ MongoDB     │ │ GitHub API  │ │ Telegram    │
   │ Atlas       │ │ (REST +     │ │ Bot API     │
   │ (external)  │ │  GraphQL)   │ │             │
   └─────────────┘ └─────────────┘ └─────────────┘
```

---

## Before Deployment Checklist

- [ ] Telegram Bot Token configured (from @BotFather)
- [ ] External MongoDB configured (MongoDB Atlas recommended)
- [ ] Encryption key generated (`openssl rand -hex 32`)
- [ ] Session secret generated (`openssl rand -hex 32`)
- [ ] GitHub OAuth App configured (if using OAuth)
- [ ] `PUBLIC_BASE_URL` determined (`https://<your-app>.herokuapp.com`)
- [ ] Webhook secret generated (`openssl rand -hex 32`)
- [ ] Heroku CLI installed and authenticated (`heroku login`)
- [ ] Git repository initialised with the project committed

---

## Step-by-Step Deployment

### Step 1: Create a Heroku application

```bash
heroku create swaggymusic-github-bot
heroku stack:set container -a swaggymusic-github-bot
```

The `container` stack tells Heroku to use the `heroku.yml` manifest and
build the Docker image from the existing `Dockerfile`.

> If the name `swaggymusic-github-bot` is taken, choose another. The app
> name determines your `PUBLIC_BASE_URL`: `https://<app-name>.herokuapp.com`.

### Step 2: Connect the Git repository

```bash
cd swaggymusic-github-bot
git init
git add .
git commit -m "Initial commit"
heroku git:remote -a swaggymusic-github-bot
```

Alternatively, connect the GitHub repository via the Heroku Dashboard
("Deploy" → "Deployment method" → "GitHub") for automatic deploys on push.

### Step 3: Configure all Config Vars

Set every required environment variable via the Heroku CLI. **Replace
the placeholder values with your real secrets.**

```bash
# Telegram
heroku config:set TELEGRAM_BOT_TOKEN='123456:ABC-DEF...' -a swaggymusic-github-bot
heroku config:set BOT_OWNER_IDS='123456789,987654321' -a swaggymusic-github-bot

# MongoDB (external — Atlas)
heroku config:set MONGODB_URI='mongodb+srv://user:pass@cluster.xxxxx.mongodb.net/swaggymusic?retryWrites=true&w=majority' -a swaggymusic-github-bot
heroku config:set MONGODB_DATABASE='swaggymusic_github_bot' -a swaggymusic-github-bot

# GitHub
heroku config:set GITHUB_API_URL='https://api.github.com' -a swaggymusic-github-bot
heroku config:set GITHUB_CLIENT_ID='Iv1.xxxxxxxx' -a swaggymusic-github-bot
heroku config:set GITHUB_CLIENT_SECRET='xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx' -a swaggymusic-github-bot
heroku config:set GITHUB_WEBHOOK_SECRET='$(openssl rand -hex 32)' -a swaggymusic-github-bot

# Security (generate strong random values)
heroku config:set ENCRYPTION_KEY='$(openssl rand -hex 32)' -a swaggymusic-github-bot
heroku config:set SESSION_SECRET='$(openssl rand -hex 32)' -a swaggymusic-github-bot

# Server
heroku config:set PUBLIC_BASE_URL='https://swaggymusic-github-bot.herokuapp.com' -a swaggymusic-github-bot
heroku config:set LOG_LEVEL='info' -a swaggymusic-github-bot

# OAuth callback (derived from PUBLIC_BASE_URL)
heroku config:set GITHUB_OAUTH_CALLBACK_URL='https://swaggymusic-github-bot.herokuapp.com/oauth/callback' -a swaggymusic-github-bot
```

> **Note:** Heroku automatically provides the `PORT` env var. Do NOT set
> it manually — Heroku assigns it at runtime.

> **Optional vars** (leave unset if not needed):
> - `GITHUB_TOKEN` — server-level PAT (currently unused by any command)
> - `GITHUB_REPO_URL` — default repo for server-level operations
> - `GITHUB_ENTERPRISE_ALLOWLIST` — comma-separated Enterprise hosts
> - `RATE_LIMIT_COMMANDS_PER_MIN` (default 20)
> - `RATE_LIMIT_GITHUB_PER_MIN` (default 60)

### Step 4: Configure external MongoDB

The bot does NOT run MongoDB inside the Heroku dyno. Use an external
MongoDB provider. **MongoDB Atlas** is recommended (free M0 tier is
sufficient for testing).

#### MongoDB Atlas setup

1. Create a free account at https://www.mongodb.com/atlas
2. Create a new project (e.g. "SWAGGYMUSIC")
3. Build a cluster (M0 Free Tier is fine for testing; M10+ for production)
4. Under "Database Access", create a user:
   - Username: `swaggymusic`
   - Password: generate a strong password and save it
5. Under "Network Access", add `0.0.0.0/0` (allow from anywhere — Heroku
   dynos have dynamic IPs). For production, consider Heroku Private Spaces
   + Atlas VPC peering.
6. Click "Connect" → "Drivers" → copy the connection string:
   ```
   mongodb+srv://swaggymusic:<password>@cluster0.xxxxx.mongodb.net/?retryWrites=true&w=majority
   ```
7. Replace `<password>` with your actual password.
8. Append the database name to the URI (or set `MONGODB_DATABASE` separately):
   ```
   mongodb+srv://swaggymusic:<password>@cluster0.xxxxx.mongodb.net/swaggymusic_github_bot?retryWrites=true&w=majority
   ```
9. Set the Heroku config vars:
   ```bash
   heroku config:set MONGODB_URI='mongodb+srv://swaggymusic:<password>@cluster0.xxxxx.mongodb.net/swaggymusic_github_bot?retryWrites=true&w=majority' -a swaggymusic-github-bot
   heroku config:set MONGODB_DATABASE='swaggymusic_github_bot' -a swaggymusic-github-bot
   ```

The bot validates the MongoDB connection at startup and fails fast with a
clear error if it cannot connect. MongoDB credentials are never logged
(the URI is redacted to `mongodb://user:***@host` in log output).

#### Reconnect handling

The MongoDB Go driver automatically reconnects when the connection drops.
The bot does not need explicit reconnect logic — the driver handles it
transparently. If a reconnect fails permanently, GitHub API calls will
return errors which are surfaced to the user in Telegram.

### Step 5: Configure PUBLIC_BASE_URL

```bash
heroku config:set PUBLIC_BASE_URL='https://swaggymusic-github-bot.herokuapp.com' -a swaggymusic-github-bot
```

`PUBLIC_BASE_URL` is used for:
- GitHub OAuth callback URL: `{PUBLIC_BASE_URL}/oauth/callback`
- GitHub webhook URLs: `{PUBLIC_BASE_URL}/webhook/{route-id}`
- Public HTTP endpoints

**Never use `localhost` in production.** Heroku's HTTPS endpoint is
`https://<app-name>.herokuapp.com`.

### Step 6: Configure GitHub OAuth callback

1. Go to https://github.com/settings/developers
2. Click your OAuth App (or "New OAuth App")
3. Set:
   - **Homepage URL**: `https://swaggymusic-github-bot.herokuapp.com`
   - **Authorization callback URL**:
     `https://swaggymusic-github-bot.herokuapp.com/oauth/callback`
4. Save.
5. Copy the Client ID and Client Secret to Heroku Config Vars (Step 3).

The bot's OAuth callback route is `/oauth/callback` (verified in
`cmd/bot/main.go` line 152: `mux.HandleFunc("/oauth/callback", cbServer.Handler)`).

### Step 7: Deploy

```bash
git push heroku main
```

Heroku will:
1. Detect `heroku.yml`
2. Build the Docker image using the existing `Dockerfile` (multi-stage:
   `golang:1.23-alpine` → `alpine:3.20`)
3. Run the container with all Config Vars as environment variables
4. Assign a dynamic `PORT` and route traffic to it

Watch the build logs:
```bash
heroku logs --tail -a swaggymusic-github-bot
```

### Step 8: Verify /health

```bash
curl https://swaggymusic-github-bot.herokuapp.com/health
```

Expected response (HTTP 200):
```json
{
  "status": "ok",
  "service": "swaggymusic-github-bot"
}
```

If the response is not 200, check the logs:
```bash
heroku logs --tail -a swaggymusic-github-bot
```

Common startup failures:
- Missing required env var → bot exits with `missing or invalid required environment variables: ...`
- MongoDB unreachable → bot exits with `database: ping: ...`
- Wrong `ENCRYPTION_KEY` format → bot exits with `ENCRYPTION_KEY (must be 64 hex chars => 32 bytes)`

---

## After Deployment Checklist

- [ ] Application starts successfully (check `heroku ps` — should show `web.1: up`)
- [ ] `/health` returns 200 (`curl https://<app>.herokuapp.com/health`)
- [ ] Telegram bot responds (send `/start` to the bot in Telegram)
- [ ] MongoDB connects (check logs for "MongoDB connected: mongodb://user:***@...")
- [ ] GitHub OAuth callback works (send `/connect` in private chat, complete OAuth flow)
- [ ] Webhook signature validation works (link a repo via `/addrepo`, push a commit, verify notification arrives in Telegram)

---

## Heroku Config Vars Reference

| Variable | Required | Example | Notes |
|----------|----------|---------|-------|
| `TELEGRAM_BOT_TOKEN` | ✅ | `123456:ABC-DEF...` | From @BotFather |
| `BOT_OWNER_IDS` | ✅ | `123456789,987654321` | Comma-separated Telegram user IDs |
| `MONGODB_URI` | ✅ | `mongodb+srv://...` | External MongoDB (Atlas) |
| `MONGODB_DATABASE` | ✅ | `swaggymusic_github_bot` | Database name |
| `GITHUB_API_URL` | ✅ | `https://api.github.com` | Default for github.com |
| `GITHUB_CLIENT_ID` | Optional | `Iv1.xxxxxxxx` | Required for OAuth |
| `GITHUB_CLIENT_SECRET` | Optional | `xxxx...` | Required for OAuth |
| `GITHUB_OAUTH_CALLBACK_URL` | Optional | `https://<app>.herokuapp.com/oauth/callback` | Required for OAuth |
| `GITHUB_WEBHOOK_SECRET` | ✅ | (64 hex chars) | `openssl rand -hex 32` |
| `ENCRYPTION_KEY` | ✅ | (64 hex chars) | `openssl rand -hex 32` |
| `SESSION_SECRET` | ✅ | (64 hex chars) | `openssl rand -hex 32` |
| `PUBLIC_BASE_URL` | ✅ | `https://<app>.herokuapp.com` | HTTPS in production |
| `PORT` | Auto | (set by Heroku) | Do NOT set manually |
| `LOG_LEVEL` | Optional | `info` | `debug`/`info`/`warn`/`error` |
| `GITHUB_TOKEN` | Optional | (unused) | Server-level PAT (reserved) |
| `GITHUB_REPO_URL` | Optional | (unused) | Default repo (reserved) |
| `GITHUB_ENTERPRISE_ALLOWLIST` | Optional | `github.example.com` | For GitHub Enterprise |
| `RATE_LIMIT_COMMANDS_PER_MIN` | Optional | `20` | Default 20 |
| `RATE_LIMIT_GITHUB_PER_MIN` | Optional | `60` | Default 60 |

---

## Graceful Shutdown

Heroku sends `SIGTERM` before shutting down a dyno (e.g. on restart,
scale-down, or new deploy). The bot handles `SIGTERM` correctly:

1. **Receives SIGTERM** (signal.Notify in `cmd/bot/main.go` line 195)
2. **Stops Telegram updates** (`bot.Stop()` + `updateCancel()`)
3. **Shuts down HTTP server** (`httpServer.Shutdown()` with 10s timeout —
   in-flight requests are allowed to finish)
4. **Disconnects MongoDB** (`db.Disconnect()` with 10s timeout)
5. **Exits cleanly**

Heroku waits up to 30 seconds after `SIGTERM` before `SIGKILL`. The bot's
10-second shutdown timeout leaves a comfortable margin.

---

## Security Notes

- **No secrets in source code**: all secrets come from Heroku Config Vars.
- **No `.env` file in the image**: the Dockerfile does not `COPY .env`.
- **`.gitignore` excludes `.env`**: prevents accidental commits.
- **MongoDB not exposed**: the bot only connects outbound to MongoDB; it
  does not run MongoDB locally.
- **Webhook signature validation enabled**: all incoming webhooks are
  verified via HMAC-SHA-256.
- **HTTPS enforced**: `PUBLIC_BASE_URL` must be HTTPS in production
  (validated at startup).
- **No debug endpoints**: `/health` returns only `{"status":"ok"}` — no
  config, no secrets, no DB credentials.

---

## Troubleshooting

### App crashes on startup with "missing required environment variables"
- Run `heroku config -a <app>` and verify every required var is set.
- Common miss: `GITHUB_OAUTH_CALLBACK_URL` is required if `GITHUB_CLIENT_ID` is set.

### App crashes with "database: ping: ..."
- Verify `MONGODB_URI` is correct (test with `mongo "mongodb+srv://..."` locally).
- Verify MongoDB Atlas Network Access allows `0.0.0.0/0` (or your Heroku
  Private Space IPs).
- Verify the database user password is correct.

### `/health` returns 503 or connection refused
- Run `heroku ps -a <app>` — is the dyno up?
- Run `heroku logs --tail -a <app>` — check for startup errors.
- The dyno may be in a crash loop; check `heroku releases -a <app>`.

### Telegram bot doesn't respond
- Verify `TELEGRAM_BOT_TOKEN` is correct.
- Send `/start` to the bot in Telegram.
- Check logs for "Telegram bot started: @<username>".
- Ensure the bot is not in "privacy mode" (use `/setprivacy` with @BotFather).

### GitHub OAuth callback returns "Invalid state"
- Verify `GITHUB_OAUTH_CALLBACK_URL` matches `PUBLIC_BASE_URL + /oauth/callback`.
- States expire after 10 minutes; retry `/connect`.

### Webhooks return 401
- `GITHUB_WEBHOOK_SECRET` in Heroku must match the secret configured on
  the GitHub repository webhook. The bot sets this automatically when
  creating webhooks via `/addrepo`.

### Dyno sleeps (free tier)
- Heroku free tier dynos sleep after 30 minutes of inactivity. The bot
  will stop responding until the next HTTP request wakes it.
- For production, use at least a `basic` dyno (`heroku ps:scale web=1:basic -a <app>`).

---

## Useful Heroku Commands

```bash
# View logs
heroku logs --tail -a swaggymusic-github-bot

# View config vars
heroku config -a swaggymusic-github-bot

# Restart the dyno
heroku restart -a swaggymusic-github-bot

# Scale to 1 dyno (basic tier)
heroku ps:scale web=1:basic -a swaggymusic-github-bot

# Open the app in a browser
heroku open -a swaggymusic-github-bot

# Run a one-off command in the dyno
heroku run bash -a swaggymusic-github-bot
```
