# SWAGGYMUSIC GitHub Controller Bot

A production-quality Telegram bot that lets authorised users manage GitHub
repositories, issues, pull requests, GitHub Actions, releases, and webhooks
directly from Telegram. Built as a clean, independent rebuild owned entirely
by **SWAGGYMUSIC**.

> **Status:** All listed commands are implemented and the project compiles
> cleanly with `go build ./...`. Unit tests cover the critical security
> surfaces (encryption, OAuth state, webhook signature verification, input
> validation, rate limiting, config validation). See
> [FEATURE_PARITY.md](FEATURE_PARITY.md) for a feature-by-feature breakdown
> and known limitations.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [Features](#3-features)
4. [Requirements](#4-requirements)
5. [Installation](#5-installation)
6. [Environment Setup](#6-environment-setup)
7. [MongoDB Setup](#7-mongodb-setup)
8. [Telegram Bot Setup](#8-telegram-bot-setup)
9. [GitHub OAuth Setup](#9-github-oauth-setup)
10. [GitHub PAT Setup](#10-github-pat-setup)
11. [GitHub Webhook Setup](#11-github-webhook-setup)
12. [Docker Deployment](#12-docker-deployment)
12.5. [Deploy to Heroku](#125-deploy-to-heroku)
13. [Command List](#13-command-list)
14. [Permission Model](#14-permission-model)
15. [Security Model](#15-security-model)
16. [Backup Guidance](#16-backup-guidance)
17. [Troubleshooting](#17-troubleshooting)

---

## 1. Overview

The SWAGGYMUSIC GitHub Controller Bot is a Go service that bridges Telegram
and GitHub. Once a user connects their GitHub account (via OAuth or a
Personal Access Token), they can manage repositories, create issues, review
and merge pull requests, trigger workflow runs, create releases, and receive
real-time notifications for GitHub events — all from Telegram.

The bot is designed for teams that want a single, secure interface for
common GitHub operations without leaving their group chat. It enforces
strict separation between Telegram-side and GitHub-side permissions:
Telegram admin status never automatically grants GitHub repository access.
Every GitHub operation is independently authorised via the calling user's
stored GitHub credentials, and the GitHub API itself enforces repository
permissions.

### Independent ownership

This project is owned entirely by SWAGGYMUSIC. It does **not** depend on any
package, library, or service owned by the original reference repository's
author. All third-party dependencies are widely-used, community-maintained
open-source packages from Google, the Go team, and the Telegram Bot API
community.

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         cmd/bot/main.go                         │
│   - Loads config from env (or .env)                             │
│   - Connects to MongoDB                                        │
│   - Starts Telegram long-polling                               │
│   - Starts HTTP server (OAuth callback + GitHub webhooks)      │
│   - Graceful shutdown on SIGINT/SIGTERM                        │
└──────────────────────────┬──────────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        ▼                  ▼                  ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────────┐
│  Telegram    │  │   GitHub     │  │    Webhook       │
│  Updates     │  │   OAuth      │  │    Server        │
│  (long poll) │  │  Callback    │  │  /webhook/{tok}  │
└──────┬───────┘  └──────┬───────┘  └────────┬─────────┘
       │                 │                   │
       ▼                 ▼                   ▼
┌─────────────────────────────────────────────────────────────────┐
│                       commands.Dispatcher                       │
│  - Routes /command → Handler                                    │
│  - Routes reply → replyctx.Handler (forwards to GitHub)         │
│  - Routes callback query → settings/action/access handler      │
│  - Enforces: rate limit, owner-only, admin-only, private-only   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        ▼                  ▼                  ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────────┐
│  ghaccess    │  │   ghops      │  │   permissions    │
│  (auth +     │  │  (GitHub     │  │  (Telegram role  │
│   encrypt)   │  │   operations)│  │   checks)        │
└──────────────┘  └──────────────┘  └──────────────────┘
        │                 │
        ▼                 ▼
┌──────────────┐  ┌──────────────────┐
│  encryption  │  │   github (client │
│  (AES-256-   │  │   factory, OAuth │
│   GCM)       │  │   signature)     │
└──────────────┘  └──────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│                     database (MongoDB v2)                       │
│  Collections: users, github_accounts, chats, oauth_states,     │
│               webhooks, message_contexts, audit_logs           │
└─────────────────────────────────────────────────────────────────┘
```

### Package layout

```
swaggymusic-github-bot/
├── cmd/bot/                      # main entry point
├── internal/
│   ├── auth/                     # OAuth callback HTTP handler
│   ├── audit/                    # Audit logging service
│   ├── cache/                    # Generic in-memory TTL cache
│   ├── commands/                 # Telegram command dispatcher + handlers
│   ├── config/                   # Env-var loading + validation
│   ├── database/                 # MongoDB wrapper + indexes
│   ├── encryption/               # AES-256-GCM encrypt/decrypt
│   ├── ghaccess/                 # GitHub Access panel (OAuth/PAT lifecycle)
│   ├── github/                   # GitHub client factory, OAuth, signature
│   ├── ghops/                    # Higher-level GitHub operations
│   ├── logger/                   # Leveled structured logger
│   ├── models/                   # MongoDB document structs
│   ├── permissions/              # Telegram role checks (owner/admin/normal)
│   ├── ratelimit/                # Per-key token-bucket rate limiter
│   ├── replyctx/                 # Reply-to-GitHub forwarding
│   ├── telegram/                 # tgbotapi wrapper
│   ├── validation/               # Input validation (SSRF, path traversal, etc.)
│   └── webhooks/                 # Webhook HTTP server + event formatters
├── .github/workflows/ci.yml      # CI: fmt, vet, build, test, docker build
├── Dockerfile                    # Multi-stage Alpine build
├── docker-compose.yml            # bot + MongoDB (not exposed publicly)
├── .env.example                  # Placeholder env vars (NO real secrets)
├── go.mod / go.sum               # Pinned dependencies
├── LICENSE                       # MIT (SWAGGYMUSIC)
├── README.md                     # This file
├── SECURITY_AUDIT.md             # Security findings + improvements
└── FEATURE_PARITY.md             # Reference vs. SWAGGYMUSIC feature matrix
```

---

## 3. Features

### Authentication
- **GitHub OAuth App** flow with single-use, expiring state tokens
- **Personal Access Token** (PAT) storage with AES-256-GCM encryption at rest
- **GitHub Enterprise Server** URL support (allowlist-gated, HTTPS-enforced)
- **GitHub Access panel** (`/ghaccess`) with connect/test/replace/disconnect buttons
- **Never display** stored tokens after submission — only "Configured"

### Repository Management
- List, link, unlink, browse, star/unstar, watch/unwatch, fork, archive/unarchive
- View contributors, languages, statistics, branches, default branch

### File Management
- Browse directories, view text files, create/delete files
- Path traversal prevention, no shell execution, no local filesystem access

### Branch Management
- List, view, create, delete, change default branch

### Issue Management
- Create, view, comment, close/reopen, assign/unassign, labels, milestones,
  lock/unlock
- Pin / unpin issues (via GitHub GraphQL API)

### Pull Request Management
- List, view commits/files/diff, reviews, checks, mergeable status
- Approve, request changes, merge (with confirmation button)
- Merge methods: merge, squash, rebase
- Request reviewers
- Convert PR to draft / mark ready for review (via GitHub GraphQL API)

### Discussions (new)
- List discussions, create discussions, mark discussion comments as answers
- All via GitHub GraphQL API (requires Discussions enabled on the repo)

### GitHub Actions
- List workflows and runs, dispatch, rerun, cancel, get logs URL

### Releases
- List, view latest, create with auto-generated notes, generate changelog

### Webhooks
- Receives 40+ GitHub event types, validates HMAC-SHA-256 signatures
- Forwards events as HTML-formatted Telegram messages
- Stateless webhook routing via encrypted path tokens

### Telegram Notifications
- Reply to a notification → posts the reply as a GitHub comment
- Mute/unmute repositories per chat
- Per-repo event filtering

### Security
- AES-256-GCM encrypted token storage
- OAuth state validation (single-use, 10-minute expiry)
- Webhook signature verification (constant-time comparison)
- SSRF prevention on GitHub API URL configuration
- Path traversal prevention on file operations
- Per-user rate limiting on commands and GitHub API calls
- Audit logging of security-sensitive actions
- Strict separation of Telegram vs. GitHub permissions
- Owner credentials isolated from user credentials

---

## 4. Requirements

- **Go**: 1.23 or higher (tested with 1.23.4)
- **MongoDB**: 7.0+ (provided via docker-compose)
- **Docker & Docker Compose**: for containerised deployment
- **Telegram Bot Token**: from [@BotFather](https://t.me/BotFather)
- **GitHub OAuth App** (optional — PAT-only mode also supported)
- **Public HTTPS URL**: for OAuth callback and GitHub webhooks

---

## 5. Installation

### Option A: Docker Compose (Recommended)

```bash
git clone <your-repo-url> swaggymusic-github-bot
cd swaggymusic-github-bot
cp .env.example .env
# Edit .env with your real values
docker compose up -d --build
```

### Option B: Manual Build

```bash
go mod download
go build -o swaggymusic-bot ./cmd/bot
./swaggymusic-bot
```

---

## 6. Environment Setup

Copy `.env.example` to `.env` and fill in real values. Critical variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | ✅ | From @BotFather |
| `BOT_OWNER_IDS` | ✅ | Comma-separated Telegram user IDs |
| `MONGODB_URI` | ✅ | MongoDB connection string |
| `MONGODB_DATABASE` | ✅ | Database name |
| `GITHUB_WEBHOOK_SECRET` | ✅ | Random 32+ char string |
| `ENCRYPTION_KEY` | ✅ | 64-char hex string (32 bytes) |
| `SESSION_SECRET` | ✅ | Random string for session signing |
| `PUBLIC_BASE_URL` | ✅ | HTTPS URL of your bot server |
| `GITHUB_CLIENT_ID` | Optional | OAuth App client ID |
| `GITHUB_CLIENT_SECRET` | Optional | OAuth App client secret |
| `GITHUB_OAUTH_CALLBACK_URL` | Optional | Defaults to `{PUBLIC_BASE_URL}/oauth/callback` |
| `GITHUB_API_URL` | Optional | Defaults to `https://api.github.com` |
| `GITHUB_TOKEN` | Optional | Server-level PAT (owner-only) |
| `GITHUB_ENTERPRISE_ALLOWLIST` | Optional | Comma-separated Enterprise hosts |
| `PORT` | Optional | Defaults to `8080` |
| `LOG_LEVEL` | Optional | `debug`/`info`/`warn`/`error` (default `info`) |

Generate strong secrets with:
```bash
openssl rand -hex 32   # ENCRYPTION_KEY, SESSION_SECRET, GITHUB_WEBHOOK_SECRET
```

---

## 7. MongoDB Setup

### Via docker-compose (recommended)

The `docker-compose.yml` provisions MongoDB 7 with:
- A persistent named volume (`mongo_data`)
- Authentication enabled (`MONGO_USER` / `MONGO_PASSWORD`)
- **Not exposed publicly** (only reachable from the bot service via the
  internal Docker network)

Update `MONGODB_URI` in `.env` to match:
```
MONGODB_URI=mongodb://swaggymusic:change_me_in_production@mongo:27017
```

### Standalone MongoDB

If you run MongoDB separately, ensure:
- Authentication is enabled
- The connection string in `MONGODB_URI` includes credentials
- MongoDB is not exposed to the public internet
- You use MongoDB 7.0+ (older versions may work but are untested)

### Indexes

The bot creates the following indexes automatically on startup:
- `github_accounts`: unique on `(telegram_id, github_user_id)`
- `oauth_states`: TTL on `expires_at` (auto-expires after 15 minutes)
- `chats`: on `links.repo_full_name` (for reverse lookup)
- `webhooks`: unique on `(chat_id, repo_full_name)`
- `message_contexts`: unique on `(chat_id, message_id)`, TTL on `expires_at`
- `audit_logs`: on `created_at` (descending) and `(actor_id, created_at)`

---

## 8. Telegram Bot Setup

1. Open [@BotFather](https://t.me/BotFather) in Telegram.
2. Send `/newbot` and follow the prompts.
3. Copy the bot token to `TELEGRAM_BOT_TOKEN` in `.env`.
4. Send `/setprivacy` → choose `Disable` if you want the bot to see all
   messages in groups (needed for reply-to-GitHub to work on non-command
   replies). Otherwise, the bot only sees commands and replies.
5. Get your own Telegram user ID (e.g. via [@userinfobot](https://t.me/userinfobot))
   and add it to `BOT_OWNER_IDS`.

---

## 9. GitHub OAuth Setup

1. Go to https://github.com/settings/developers
2. Click "New OAuth App"
3. Fill in:
   - **Application name**: SWAGGYMUSIC GitHub Bot
   - **Homepage URL**: your `PUBLIC_BASE_URL`
   - **Authorization callback URL**: `{PUBLIC_BASE_URL}/oauth/callback`
4. Copy the **Client ID** to `GITHUB_CLIENT_ID`.
5. Generate a **Client Secret** and copy it to `GITHUB_CLIENT_SECRET`.
6. Set `GITHUB_OAUTH_CALLBACK_URL` to `{PUBLIC_BASE_URL}/oauth/callback`
   (or leave blank to auto-derive).

Users can now use `/connect` in a private chat with the bot to link their
GitHub account via OAuth.

---

## 10. GitHub PAT Setup

If you don't want to set up OAuth (or want users to connect via PAT):

1. Each user creates a Personal Access Token at
   https://github.com/settings/tokens with the following scopes:
   - `repo` (full repo access)
   - `admin:repo_hook` (to create webhooks)
   - `read:user`
2. Users send `/addtoken <PAT>` in a **private** chat with the bot.
3. The bot validates the token against the GitHub API, encrypts it with
   AES-256-GCM, stores the ciphertext in MongoDB, and **deletes the user's
   message** so the plaintext token doesn't linger in chat history.

Stored tokens can never be retrieved as plaintext — only replaced or
revoked.

---

## 11. GitHub Webhook Setup

Webhooks are created **automatically** when a user links a repository with
`/addrepo owner/repo`. The bot:

1. Encrypts the target chat ID (+ optional topic ID) into an opaque token.
2. Creates a repository webhook pointing to
   `{PUBLIC_BASE_URL}/webhook/{encrypted_token}`.
3. Configures the webhook with the `GITHUB_WEBHOOK_SECRET` so signatures
   can be verified.
4. Subscribes the webhook to a curated set of events (push, PR, issues,
   releases, workflow runs, etc.).

When GitHub delivers a webhook, the bot:
1. Decrypts the path token to find the target chat.
2. Verifies the `X-Hub-Signature-256` header against the body using
   constant-time comparison.
3. Parses the event and forwards it to the chat as an HTML-formatted
   message.
4. Stores a `MessageContext` mapping the Telegram message ID back to the
   GitHub object (issue/PR/comment), so replies can be forwarded back.

---

## 12. Docker Deployment

The included `Dockerfile` uses a multi-stage build:
- **Builder**: `golang:1.23-alpine`, compiles a static binary
- **Runtime**: `alpine:3.20`, runs as non-root user `swaggymusic`, includes
  `ca-certificates` and `tzdata`

The included `docker-compose.yml` provisions:
- The bot service (built from `Dockerfile`)
- A MongoDB 7 service with auth, persistent volume, no public port exposure
- A private bridge network between them
- Health checks for both services

```bash
docker compose up -d --build
docker compose logs -f bot
docker compose down      # stop
docker compose down -v   # stop + delete MongoDB volume (IRREVERSIBLE)
```

---

## 12.5. Deploy to Heroku

The project includes a `heroku.yml` manifest that tells Heroku to build
and run the application using the existing `Dockerfile` (container stack).
**No Procfile is needed** — `heroku.yml` + `Dockerfile` is sufficient.

### Prerequisites
- Heroku CLI installed (`heroku login`)
- External MongoDB (MongoDB Atlas recommended — the bot does NOT run
  MongoDB inside the Heroku dyno)
- Telegram bot token (from @BotFather)

### Quick deploy

```bash
# 1. Create the Heroku app and set the container stack
heroku create swaggymusic-github-bot
heroku stack:set container -a swaggymusic-github-bot

# 2. Set all required Config Vars (replace placeholders with real values)
heroku config:set TELEGRAM_BOT_TOKEN='123456:ABC-DEF...' -a swaggymusic-github-bot
heroku config:set BOT_OWNER_IDS='123456789' -a swaggymusic-github-bot
heroku config:set MONGODB_URI='mongodb+srv://user:pass@cluster.mongodb.net/db' -a swaggymusic-github-bot
heroku config:set MONGODB_DATABASE='swaggymusic_github_bot' -a swaggymusic-github-bot
heroku config:set GITHUB_API_URL='https://api.github.com' -a swaggymusic-github-bot
heroku config:set GITHUB_CLIENT_ID='Iv1.xxxxxxxx' -a swaggymusic-github-bot
heroku config:set GITHUB_CLIENT_SECRET='xxxx...' -a swaggymusic-github-bot
heroku config:set GITHUB_OAUTH_CALLBACK_URL='https://swaggymusic-github-bot.herokuapp.com/oauth/callback' -a swaggymusic-github-bot
heroku config:set GITHUB_WEBHOOK_SECRET="$(openssl rand -hex 32)" -a swaggymusic-github-bot
heroku config:set ENCRYPTION_KEY="$(openssl rand -hex 32)" -a swaggymusic-github-bot
heroku config:set SESSION_SECRET="$(openssl rand -hex 32)" -a swaggymusic-github-bot
heroku config:set PUBLIC_BASE_URL='https://swaggymusic-github-bot.herokuapp.com' -a swaggymusic-github-bot
heroku config:set LOG_LEVEL='info' -a swaggymusic-github-bot

# 3. Deploy
git push heroku main

# 4. Verify
curl https://swaggymusic-github-bot.herokuapp.com/health
# Expected: {"status":"ok","service":"swaggymusic-github-bot"}
```

> **Note:** Heroku automatically provides the `PORT` env var. Do NOT set
> it manually — Heroku assigns it at runtime and the bot reads it from the
> environment.

### MongoDB on Heroku

The bot connects to an **external** MongoDB instance. The recommended
provider is **MongoDB Atlas** (free M0 tier is sufficient for testing):

1. Create a cluster at https://www.mongodb.com/atlas
2. Create a database user
3. Add `0.0.0.0/0` to Network Access (Heroku dynos have dynamic IPs)
4. Copy the connection string into the `MONGODB_URI` Heroku Config Var

The bot validates the MongoDB connection at startup and fails fast if it
cannot connect. MongoDB credentials are never logged — the URI is redacted
to `mongodb://user:***@host` in log output.

### Health check

The bot exposes a lightweight `/health` endpoint (no auth, no secrets, no
GitHub calls):

```bash
curl https://swaggymusic-github-bot.herokuapp.com/health
# {"status":"ok","service":"swaggymusic-github-bot"}
```

### Graceful shutdown

Heroku sends `SIGTERM` before shutting down a dyno. The bot:
1. Receives `SIGTERM`
2. Stops accepting new Telegram updates
3. Shuts down the HTTP server (in-flight requests get up to 10 seconds)
4. Disconnects MongoDB
5. Exits cleanly

### Full deployment guide

See [HEROKU_DEPLOYMENT.md](HEROKU_DEPLOYMENT.md) for a complete
step-by-step checklist including MongoDB Atlas setup, GitHub OAuth
configuration, and troubleshooting.

---

## 13. Command List

### Authentication
| Command | Description |
|---------|-------------|
| `/start` | Show welcome message |
| `/help` | Show all commands |
| `/connect` | Connect GitHub via OAuth (private chat only) |
| `/addtoken <PAT>` | Add a GitHub Personal Access Token (private chat only) |
| `/replacetoken <new_PAT>` | Replace your stored token |
| `/testconnection` | Verify your stored token works |
| `/disconnect` | Disconnect all GitHub accounts |
| `/me` | Show your connected account |
| `/ghaccess` | Open the GitHub Access panel |
| `/configureapi <url>` | Configure the GitHub API URL |

### Repository Management
| Command | Description |
|---------|-------------|
| `/addrepo [owner/repo]` | Link a repository (admin in groups) |
| `/removerepo owner/repo` | Unlink a repository |
| `/repos` | List linked repositories |
| `/repo [owner/repo]` | Show repository info |
| `/star`, `/unstar` | Star / unstar the repo |
| `/watch`, `/unwatch` | Watch / unwatch the repo |
| `/fork` | Fork the repo |
| `/archive`, `/unarchive` | Archive / unarchive (admin) |
| `/contributors` | Top contributors |
| `/languages` | Language breakdown |
| `/stats` | Repository statistics |

### Branches
| Command | Description |
|---------|-------------|
| `/branches` | List branches |
| `/branch <name>` | Show branch info |
| `/createbranch <new> <from>` | Create a branch |
| `/deletebranch <name>` | Delete a branch (admin) |
| `/default <name>` | Change default branch (admin) |

### Files
| Command | Description |
|---------|-------------|
| `/ls [path]` | List directory contents |
| `/cat <path>` | View a text file |
| `/createfile <path> <msg> <content>` | Create a file |
| `/updatefile <path> [msg] <content>` | Update an existing file |
| `/deletefile <path>` | Delete a file (admin) |

### Issues
| Command | Description |
|---------|-------------|
| `/issue <title>` | Create an issue (multi-line body supported) |
| `/comment <text>` | Comment on a replied issue/PR |
| `/close` | Close issue/PR (reply to notification) |
| `/reopen` | Reopen issue/PR |
| `/assign @user` | Assign a user |
| `/assignme` | Assign yourself |
| `/unassign @user` | Unassign |
| `/label +bug -wip` | Add/remove labels |
| `/labels` | List labels |
| `/milestone <name>` | Set milestone |
| `/lock [reason]` | Lock conversation |
| `/unlock` | Unlock conversation |
| `/pin`, `/unpin` | Pin / unpin an issue (via GitHub GraphQL API) |

### Pull Requests
| Command | Description |
|---------|-------------|
| `/approve [text]` | Approve PR (reply to notification) |
| `/requestchanges [text]` | Request changes |
| `/merge [squash\|rebase\|merge]` | Merge PR (with confirmation) |
| `/draft` | Convert PR to draft (via GitHub GraphQL API) |
| `/ready` | Mark draft as ready for review (via GitHub GraphQL API) |
| `/checks` | Show CI status |
| `/files` | List changed files |
| `/diff` | Show change summary |
| `/reviews` | List reviews |
| `/mergeable` | Check mergeability |
| `/request @user` | Request reviewer |

### Commits
| Command | Description |
|---------|-------------|
| `/commit <SHA>` | Show commit details |
| `/commits [branch]` | Recent commits |
| `/compare <base> <head>` | Compare refs |

### GitHub Actions
| Command | Description |
|---------|-------------|
| `/actions` | List recent runs |
| `/run <workflow.yml> [branch]` | Trigger a workflow |
| `/rerun <run_id>` | Rerun failed jobs |
| `/cancel <run_id>` | Cancel a run |
| `/logs <run_id>` | Get logs URL |

### Releases
| Command | Description |
|---------|-------------|
| `/release` | Show latest release |
| `/release create <tag>` | Create a release |
| `/changelog <tag> [prev_tag]` | Generate release notes |

### Search
| Command | Description |
|---------|-------------|
| `/find <query>` | Search issues |
| `/pr <query>` | Search pull requests |
| `/search <query>` | Search code |

### Discussions (requires Discussions enabled on the repo)
| Command | Description |
|---------|-------------|
| `/discussion <title>` | Create a discussion (multi-line body supported) |
| `/discussions` | List recent discussions |
| `/answered` | Mark a discussion comment as the answer (reply to notification) |

### Settings & System
| Command | Description |
|---------|-------------|
| `/settings`, `/config`, `/notifications` | Manage notification preferences (per-event toggles) |
| `/mute` | Mute forum topic (not supported — see notes) |
| `/reload` | Reload admin cache (admins) |
| `/privacy` | Privacy policy |
| `/help` | This message |

---

## 14. Permission Model

The bot enforces **two independent layers** of permissions:

### Telegram-side roles
- **Owner** (`BOT_OWNER_IDS`): full control, can use owner-only commands
  and reload admin cache.
- **Chat admin**: cached for 1 hour, can use admin-only commands in groups
  (e.g. `/addrepo`, `/removerepo`, `/archive`).
- **Normal user**: can use non-restricted commands.

### GitHub-side permissions
Each GitHub API call uses the calling user's stored OAuth/PAT token. The
GitHub API itself enforces repository permissions (read, write, admin).
The bot **never** uses owner credentials to perform user-requested GitHub
operations.

### Important invariant
Telegram admin status does **not** grant GitHub access. A chat admin
without a connected GitHub account cannot perform GitHub operations. A
non-admin user with a connected GitHub account can perform GitHub
operations on repositories they have access to (subject to per-command
admin-only restrictions for chat-management commands like `/addrepo`).

---

## 15. Security Model

### Encryption at rest
- Stored OAuth tokens and PATs are encrypted with **AES-256-GCM**.
- The encryption key is loaded from the `ENCRYPTION_KEY` environment
  variable (64 hex chars = 32 bytes) and **never** persisted to disk.
- A fresh random 96-bit nonce is generated per `Encrypt` call.
- Authentication tags are verified on `Decrypt` (constant-time).
- There is **no** command that retrieves stored plaintext tokens.

### OAuth state
- States are 32-char hex strings generated with `crypto/rand`.
- States are stored in MongoDB with a 10-minute expiry and a TTL index.
- States are single-use: `ConsumeOAuthState` atomically marks them as used.
- CSRF protection: the state returned by GitHub must match the state we
  saved for the requesting Telegram user.

### Webhook signatures
- `X-Hub-Signature-256` (preferred) or `X-Hub-Signature` (SHA-1 legacy)
  headers are verified against the request body using HMAC.
- Comparison uses `crypto/hmac.Equal` (constant-time).
- Requests with missing or invalid signatures are rejected with HTTP 401.
- The webhook secret is loaded from `GITHUB_WEBHOOK_SECRET` and never
  logged.

### Input validation
- All user inputs (repo names, branch names, file paths, URLs, numbers) are
  validated before being passed to the GitHub API.
- SSRF prevention: custom GitHub API URLs must be HTTPS (or localhost for
  dev) and Enterprise hosts must be in an allowlist.
- Path traversal prevention: file paths cannot contain `..`, `\`, or be
  absolute.
- No shell execution: the bot never constructs shell commands from user
  input.

### Rate limiting
- Per-user Telegram command rate limit (default 20/min).
- Per-user GitHub API call rate limit (default 60/min, also enforced by
  GitHub).

### Audit logging
- Security-sensitive actions are logged to the `audit_logs` collection:
  - GitHub connect, token replacement, disconnect
  - Repository link/unlink
  - Issue creation, PR merge
  - Workflow trigger/cancel
  - Branch deletion, default branch change
  - File create/delete
  - Permission denials
- Audit entries never include secrets, tokens, or encryption keys.

### Secrets in logs
- The logger has no special "secret redaction" because **no secret values
  are ever passed to log calls**. Plaintext tokens exist only inside
  `ghaccess.Service` methods, briefly, before being encrypted or used for
  an API call.

---

## 16. Backup Guidance

### What to back up
- **MongoDB data volume** (`mongo_data`): contains users, encrypted tokens,
  chat configs, audit logs.
- **`.env` file**: contains the encryption key, OAuth secrets, webhook
  secret. **Without the encryption key, all stored tokens are
  unrecoverable.**

### How to back up MongoDB
```bash
# Using mongodump (run on the MongoDB container or a host with access):
docker compose exec mongo mongodump --uri="$MONGODB_URI" --out=/dump

# Copy the dump out of the container:
docker compose cp mongo:/dump ./backup-$(date +%Y%m%d)
```

### Restore
```bash
docker compose cp ./backup-YYYYMMDD mongo:/dump
docker compose exec mongo mongorestore --uri="$MONGODB_URI" /dump
```

### Key rotation
To rotate the `ENCRYPTION_KEY`:
1. Decrypt all stored tokens with the old key.
2. Re-encrypt with the new key.
3. Update `ENCRYPTION_KEY` and restart the bot.

A key-rotation script is **not** included in this build — operators must
perform this manually or write a one-off script. (Tracked as a known
limitation in [FEATURE_PARITY.md](FEATURE_PARITY.md).)

---

## 17. Troubleshooting

### Bot doesn't respond to commands
- Check `docker compose logs bot` for errors.
- Verify `TELEGRAM_BOT_TOKEN` is correct.
- Verify the bot is not in "privacy mode" if you need it to see non-command
  replies (use `/setprivacy` with @BotFather).
- Make sure your Telegram user ID is in `BOT_OWNER_IDS` for owner-only
  commands.

### OAuth callback returns "Invalid state"
- States expire after 10 minutes. Restart the OAuth flow.
- If the bot was restarted between `/connect` and the callback, the state
  may have been lost (states are persisted to MongoDB, so this should not
  happen — but check MongoDB connectivity).

### Webhooks return 401 Unauthorized
- The `GITHUB_WEBHOOK_SECRET` in `.env` must match the secret configured
  on the GitHub repository webhook. The bot sets this automatically when
  creating webhooks via `/addrepo`, so if you changed the secret, you'll
  need to re-create the webhook.
- Check `docker compose logs bot` for signature verification errors.

### MongoDB connection errors
- Verify `MONGODB_URI` is reachable from the bot container.
- If using docker-compose, the URI should reference `mongo` (the service
  name), not `localhost`.
- Verify `MONGO_USER` and `MONGO_PASSWORD` match what was set on first run
  (MongoDB only applies these on initial volume creation — to change them,
  you must delete the volume and re-create).

### Token validation fails
- Ensure the PAT has the required scopes: `repo`, `admin:repo_hook`,
  `read:user`.
- For `/pin`, `/unpin`, `/draft`, `/ready`, `/discussion`, `/answered`:
  these use the GitHub GraphQL API, which requires the `repo` scope (and
  `read:discussion` / `write:discussion` for Discussions).
- For GitHub Enterprise, ensure `GITHUB_API_URL` points to
  `https://YOUR-HOST/api/v3` and the host is in
  `GITHUB_ENTERPRISE_ALLOWLIST`.

### `/discussion` or `/answered` fails
- GitHub Discussions must be enabled on the repository (Settings →
  Features → Discussions).
- Your token must have `read:discussion` (for listing) and
  `write:discussion` (for creating / marking answers) scopes.
- `/answered` must be used as a reply to a discussion comment notification
  (not an issue/PR notification).

### Forum topic targeting not working
- The underlying `go-telegram-bot-api/v5` library does not expose
  `message_thread_id` on incoming messages, so the bot cannot route
  webhooks to specific forum topics. Webhooks are delivered to the chat
  (not a specific topic). See [FEATURE_PARITY.md](FEATURE_PARITY.md).
