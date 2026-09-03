# SWAGGYMUSIC GitHub Controller Bot — Security Audit

This document records the security audit performed on the reference
repository (`GithubBot-master.zip`) and the ongoing security posture of the
SWAGGYMUSIC rebuild, including all targeted improvements made in the latest
update.

> **No real secrets appear in this document.** All values shown are
> placeholders, examples, or redacted.

---

## 1. Reference Repository Security Findings (unchanged)

The reference repository (`github-webhook` module by AshokShau) was audited
in full in the initial build. Findings are preserved here for context.

### 1.1 CRITICAL — Hardcoded Telegram API credentials
**Location:** `main.go:43` — hardcoded API ID `6` and API hash
`eb06d4abfb49dc3eeb1aeb98ae0f581e`, plus hardcoded library path
`./libtdjson.so.1.8.67`.

**SWAGGYMUSIC resolution:** Uses `go-telegram-bot-api/v5` (Bot API HTTP
interface, no API ID/hash, no TDLib binary). No hardcoded credentials.

### 1.2 HIGH — Dependency on original author's package
**Location:** `go.mod:6` — `github.com/AshokShau/gotdbot v0.9.5`.

**SWAGGYMUSIC resolution:** No dependency on any package owned by the
original author. All dependencies are community-maintained.

### 1.3 MEDIUM — MongoDB exposed publicly in docker-compose
**Location:** `docker-compose.yml:14-15` — `ports: ["27017:27017"]`, no auth.

**SWAGGYMUSIC resolution:** MongoDB not exposed publicly (uses `expose:`
not `ports:`), authentication enabled, private bridge network.

### 1.4 MEDIUM — No session secret / weak state management
**Location:** `main.go:36`, in-memory-only OAuth state cache.

**SWAGGYMUSIC resolution:** OAuth states persisted to MongoDB, single-use,
TTL-indexed.

### 1.5 LOW — Webhook secret warning instead of failure
**Location:** `internal/github/webhooks.go:80-82`.

**SWAGGYMUSIC resolution:** `GITHUB_WEBHOOK_SECRET` is required; bot refuses
to start without it.

### 1.6 LOW — No rate limiting
**SWAGGYMUSIC resolution:** Per-user rate limiters for Telegram commands
and GitHub API calls.

### 1.7 LOW — No input validation on repository names
**SWAGGYMUSIC resolution:** Comprehensive `validation` package.

---

## 2. Removed Suspicious Functionality (unchanged)

| Item | Reason |
|------|--------|
| `gotdbot` TDLib wrapper dependency | Supply-chain risk; replaced with `go-telegram-bot-api/v5` |
| Hardcoded Telegram API ID/hash | Replaced with env-var configuration |
| `libtdjson.so` runtime dependency | Not needed with Bot API approach |
| Public MongoDB port exposure | Replaced with internal-only network + auth |
| In-memory-only OAuth state cache | Replaced with MongoDB-backed atomic single-use states |
| `//go:generate go run github.com/AshokShau/gotdbot/scripts/tools` | Removed |

---

## 3. External Services Used

The SWAGGYMUSIC rebuild makes outbound network calls to:

| Endpoint | Purpose | Source |
|----------|---------|--------|
| `api.github.com` | GitHub REST API | `go-github/v66` |
| `api.github.com/graphql` | GitHub GraphQL API (pin/unpin/draft/ready/discussions) | `shurcooL/graphql` (new) |
| `github.com/login/oauth/authorize` | GitHub OAuth | `golang.org/x/oauth2` |
| `github.com/login/oauth/access_token` | GitHub OAuth token exchange | `golang.org/x/oauth2` |
| `api.telegram.org` | Telegram Bot API | `go-telegram-bot-api/v5` |

For GitHub Enterprise Server deployments, the bot also calls:
- `<enterprise-host>/api/v3` (REST API)
- `<enterprise-host>/api/graphql` (GraphQL API)

The Enterprise host must be in `GITHUB_ENTERPRISE_ALLOWLIST`.

**No other endpoints are contacted.** No telemetry, analytics, or data
exfiltration. Verified by full source code audit.

---

## 4. Credentials Removed (unchanged)

- Hardcoded Telegram API ID/hash — removed.
- Hardcoded library path — removed.
- No hardcoded bot tokens, GitHub tokens, OAuth secrets, MongoDB
  credentials, encryption keys, or webhook secrets in source.
- `.env.example` contains only empty placeholder values (no fake-looking
  secrets — cleaned in latest update).

---

## 5. Security Improvements in SWAGGYMUSIC (updated)

### Defense-in-depth (existing)
- AES-256-GCM encryption for stored tokens
- Rate limiting on Telegram commands and GitHub API calls
- Audit logging of all security-sensitive actions
- Strict input validation
- SSRF prevention on GitHub API URL configuration

### New in latest update
- **Webhook route IDs**: New webhooks use `/webhook/{random_64_char_hex}`
  instead of `/webhook/{encrypted_chat_id}`. Route IDs are stored in
  MongoDB, support rotation and revocation, and expose no chat ID in the
  URL. Backward compatible with old encrypted-token webhooks.
- **Per-event notification filtering**: Webhook `processEvent` now consults
  per-repo, per-event settings before delivering notifications. Previously
  all delivered events were sent regardless of user preferences.
- **GraphQL client**: Added `github.com/shurcooL/graphql` for
  pin/unpin/draft/ready/discussions. Token is NOT stored on the Client
  struct — it lives only inside the oauth2 transport.
- **Fixed broken settings UI**: The `c:mute:` callback button now actually
  toggles the mute flag (was a no-op). The `c:cfg:back` button now
  correctly returns to the repo list.
- **GitHub Access panel**: Now has all 8 required buttons (was missing 3).
- **File commands use repo's default branch**: No longer hard-coded to
  `"main"` (works on `master`/`develop`/etc. repos).

### Operational security (existing)
- Multi-stage Docker build with non-root runtime user
- MongoDB not exposed publicly by default
- MongoDB authentication enabled by default
- Health checks for both bot and MongoDB services
- Graceful shutdown on SIGINT/SIGTERM
- Configurable log levels

### Cryptographic hygiene (existing)
- Encryption key is exactly 32 bytes (AES-256), validated at startup
- Nonces are 96-bit, freshly random per encryption
- Webhook signatures use HMAC-SHA-256 with SHA-1 fallback
- All secret comparisons use constant-time functions

---

## 6. Latest Security Re-Audit Results

A focused security re-audit was performed covering 10 categories. **All 10
categories passed with no issues found.**

| # | Category | Result |
|---|----------|--------|
| 1 | Hardcoded secrets | ✅ No issues |
| 2 | Hardcoded owner IDs | ✅ No issues |
| 3 | Backdoors / kill switches | ✅ No issues |
| 4 | Telemetry / analytics | ✅ No issues |
| 5 | Server-token isolation | ✅ No issues (token loaded but never used by any command) |
| 6 | Token leakage in logs | ✅ No issues |
| 7 | Unsafe logging in ghaccess | ✅ No issues |
| 8 | SSRF in /addrepo | ✅ No issues |
| 9 | Path traversal in file ops | ✅ No issues (defense-in-depth at both layers) |
| 10 | Webhook signature verification | ✅ No issues (unconditional, before any parsing) |

### Detailed findings

**Hardcoded secrets**: Searched all .go files for Telegram bot tokens,
GitHub token prefixes, 64-char hex strings, and MongoDB connection strings
with passwords. No matches outside test files (which use deterministic
test vectors).

**Owner access isolation**: `cfg.GitHubToken` is referenced exactly once
outside `config.go` — in a startup log line. No command handler uses it.
The `ClientFactory` exposes no `NewServerClient` method. Every GitHub call
uses the user's decrypted token via `GetDecryptedClient` /
`GetDecryptedToken`.

**Token leakage**: The decrypted token variable (`tok` in `ghaccess.go`,
`token` in `replyctx.go` and `dispatcher.go`) is never passed to any log
call. Error messages from `encryption.Decrypt` return only sentinel errors
(no input value included).

**SSRF in /addrepo**: The webhook URL is constructed from
`d.deps.Cfg.PublicBaseURL` (env var, validated at startup) + a random route
ID or encrypted token. No user-controllable URL is ever set as a webhook
target.

**Path traversal**: `ValidateFilePath` is called at BOTH the command layer
(`internal/commands/files.go`) AND the ghops layer (`internal/ghops/files.go`).
Even if a future caller forgot to validate, the ghops layer would still
block traversal.

**Webhook signature verification**: `github.VerifyWebhookSignature` is
called unconditionally in `Handler` before any payload parsing. There is no
code path that reaches `processEvent` without first passing signature
verification.

### Minor informational notes (not vulnerabilities)
- `cmd/bot/main.go:68` log message was corrected to accurately say
  "currently unused by any command" instead of "owner-only operations".
- `internal/graphqlclient/graphqlclient.go` doc comment was corrected to
  accurately reflect that the token is NOT stored on the Client struct.
- `docker-compose.yml` default MongoDB password is `change_me_in_production`
  (obvious placeholder). Operators are expected to override via `MONGO_PASSWORD`.

---

## 7. Remaining Risks (updated)

### 7.1 Encryption key rotation is manual
No automated key-rotation script. Operators must manually decrypt all
stored tokens with the old key, re-encrypt with the new key, update
`ENCRYPTION_KEY`, and restart.

### 7.2 Forum topic targeting is limited
`go-telegram-bot-api/v5` does not expose `message_thread_id` on incoming
messages. Webhooks deliver to the chat's main feed, not a specific topic.
`/mute` returns "unsupported".

### 7.3 Server-level GitHub token unused
`GITHUB_TOKEN` is loaded but no command consumes it. Reserved for future
owner-only operations. This is actually the safest configuration (smaller
attack surface).

### 7.4 Audit logs in the same database as operational data
If an attacker gains write access to MongoDB, they could modify audit logs.
For high-security deployments, forward audit logs to a separate SIEM.

### 7.5 No distributed rate limiting
The rate limiter is in-memory and per-process. Multi-replica deployments
would need a shared rate limiter (e.g. Redis).

### 7.6 GraphQL rate limits (new)
GitHub's GraphQL API has a separate rate limit (5,000 points/hour/token).
Heavy use of `/discussions` could exhaust this faster than the REST rate
limit. Not currently tracked separately.

---

## 8. Dependency Risks (updated)

All direct dependencies are widely-used, community-maintained packages:

| Dependency | Version | Maintainer | Risk |
|------------|---------|------------|------|
| `go-telegram-bot-api/v5` | v5.5.1 | Community | Low. Note: v5 does not support forum topics. |
| `google/go-github/v66` | v66.0.0 | Google | Low. |
| `mongo-driver/v2` | v2.1.0 | MongoDB Inc. | Low. |
| `golang.org/x/oauth2` | v0.23.0 | Go team | Low. |
| `golang.org/x/crypto` | v0.28.0 | Go team | Low. |
| `joho/godotenv` | v1.5.1 | Community | Low. |
| **`shurcooL/graphql`** (new) | v0.0.0-20240915 | Community | Low. Single-purpose GraphQL client. Widely used. |

**No dependency is owned by the original reference repository author.**

Indirect dependencies are managed by `go mod` and pinned in `go.sum`. Run
`go mod verify` to confirm integrity. The CI workflow includes an optional
`govulncheck` step.

---

## 9. Deployment Recommendations (unchanged)

1. Always use HTTPS for `PUBLIC_BASE_URL`.
2. Never expose MongoDB publicly.
3. Use strong secrets: `openssl rand -hex 32`.
4. Restrict `BOT_OWNER_IDS` to trusted Telegram user IDs.
5. Set `LOG_LEVEL=info` (or `warn`) in production.
6. Back up the MongoDB volume and `.env` file regularly.
7. Keep dependencies updated.
8. Run `govulncheck ./...` periodically.
9. Monitor audit logs for suspicious activity.
10. Use a reverse proxy for TLS termination and additional rate limiting.

---

## 10. Summary

The SWAGGYMUSIC rebuild addresses all HIGH and CRITICAL findings from the
reference repository, adds defense-in-depth controls, and removes all
dependencies on the original author's packages.

The latest targeted improvement pass added:
- Webhook route IDs (no chat ID exposure in URLs)
- Per-event notification filtering (user preferences respected)
- GraphQL client for pin/unpin/draft/ready/discussions
- Fixed broken settings UI
- Completed the GitHub Access panel
- Cleaned `.env.example` of fake-looking secrets

A focused security re-audit of 10 categories found **no issues** in any
category. The project is safe to deploy in production following the
recommendations in section 9.
