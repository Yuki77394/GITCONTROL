# SWAGGYMUSIC GitHub Controller Bot — Feature Parity Report

This document compares every feature discovered in the reference repository
(`GithubBot-master.zip` by AshokShau) against the SWAGGYMUSIC rebuild, and
tracks all targeted improvements made in the latest update.

Status legend:
- **Implemented** — feature is present and works.
- **Improved** — feature is present and meaningfully better than the
  reference (e.g. added security, better error handling).
- **Removed** — feature was intentionally not carried over; reason given.
- **Unsupported** — feature cannot be implemented with the current
  dependencies (e.g. GraphQL-only endpoints).
- **Incomplete** — feature is partially implemented; gaps documented.
- **Newly Added** — feature added in the latest targeted improvement pass
  that was not in the original reference repository.

---

## 1. Authentication & Connection

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| GitHub OAuth App connect (`/connect`) | **Improved** | States persisted to MongoDB (survives restarts), single-use, 10-min expiry with TTL index. |
| Disconnect (`/disconnect`, `/logout`) | **Implemented** | Removes all GitHub accounts and encrypted tokens. |
| Show connected account (`/me`) | **Implemented** | Shows username, auth method, API URL, account count. Never shows token. |
| Private-chat-only enforcement for `/connect` | **Implemented** | `PrivateOnly: true` on the handler. |
| Personal Access Token (PAT) support | **Implemented** (new) | `/addtoken`, `/replacetoken`, `/testconnection`. Token message is deleted after submission. |
| GitHub Enterprise Server URL support | **Implemented** (new) | `GITHUB_API_URL` + allowlist. |
| GitHub Access panel | **Improved** (new) | `/ghaccess` now has all 8 required buttons: Connect, Add Token, Replace Token, Test Connection, Configure API URL, Add Repository, Select Repository, Disconnect. Also shows Active Repository. |
| Token encryption at rest (AES-256-GCM) | **Improved** | Strict 32-byte keys, constant-time tag verification. |
| Never display stored token | **Implemented** | No command retrieves plaintext tokens. Only "Configured" / "Not configured" is shown. |

---

## 2. Repository Management

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| `/addrepo [owner/repo]` | **Improved** | Now uses opaque random route IDs for webhook URLs (when route store is available). Falls back to encrypted-token URLs for backward compatibility. |
| `/removerepo [owner/repo]` | **Improved** | Also deletes the webhook route ID. |
| `/add`, `/rm` (aliases) | **Implemented** | |
| `/repos` | **Implemented** | Shows mute status. |
| `/repo [owner/repo]` | **Implemented** | Shows description, stars, forks, watchers, issues, default branch. |
| `/star`, `/unstar`, `/watch`, `/unwatch`, `/fork` | **Implemented** | |
| `/archive`, `/unarchive` | **Implemented** | Admin-only in groups. |
| `/contributors`, `/languages`, `/stats` | **Implemented** | |
| Interactive repo selection | **Implemented** | Paginated inline keyboard. |
| Webhook auto-creation on `/addrepo` | **Implemented** | |
| Webhook auto-deletion on `/removerepo` | **Implemented** | |
| Repo rename tracking | **Implemented** | `UpdateRepoLinkName` on `repository.renamed` event. |

---

## 3. Branch Management

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| `/branches`, `/branch <name>` | **Implemented** | |
| `/default <name>` | **Implemented** | Admin-only. |
| `/createbranch <new> <from>` | **Implemented** (new) | |
| `/deletebranch <name>` | **Implemented** (new) | Admin-only. |

---

## 4. File Management

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| Browse repository files (`/ls`) | **Implemented** (new) | Now uses repo's default branch (not hard-coded "main"). |
| View text file contents (`/cat`) | **Implemented** (new) | Uses default branch. |
| Create file (`/createfile`) | **Implemented** (new) | Uses default branch. |
| **Update file (`/updatefile`)** | **Newly Added** | Full implementation: validates path, fetches current SHA, calls GitHub Contents API update. Optional commit message. Content size cap. Audit-logged. |
| Delete file (`/deletefile`) | **Implemented** (new) | Admin-only. Uses default branch. |
| Path traversal prevention | **Implemented** | Defense-in-depth: validation at both command layer AND ghops layer. |
| No shell execution of repo files | **Implemented** | The bot never executes repository files locally. |

---

## 5. Issue Management

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| `/issue <title>` | **Implemented** | Multi-line body supported. |
| `/comment <text>` | **Implemented** | Uses stored message context. |
| `/close`, `/reopen` | **Implemented** | |
| `/assign @user`, `/assignme`, `/unassign @user` | **Implemented** | Username validation. |
| `/label +bug -wip` | **Implemented** | Parses `+`/`-` prefixes. |
| `/labels` | **Implemented** | |
| `/milestone <name>` | **Implemented** | Resolves milestone name to number. |
| `/lock [reason]`, `/unlock` | **Implemented** | |
| **`/pin`** | **Newly Added** (was Unsupported) | Now implemented via GitHub GraphQL `pinIssue` mutation. Requires the user's token to have `repo` scope. |
| **`/unpin`** | **Newly Added** (was Unsupported) | Now implemented via GitHub GraphQL `unpinIssue` mutation. |

---

## 6. Pull Request Management

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| `/approve [text]`, `/requestchanges [text]` | **Implemented** | |
| `/merge [squash\|rebase\|merge]` | **Improved** | Requires explicit confirmation via inline keyboard button. Checks mergeability. Audit-logged. |
| **`/draft`** | **Newly Added** (was Unsupported) | Now implemented via GitHub GraphQL `convertPullRequestToDraft` mutation. Handles already-draft PRs gracefully. |
| **`/ready`** | **Newly Added** (was Unsupported) | Now implemented via GitHub GraphQL `markPullRequestReadyForReview` mutation. Handles already-ready PRs gracefully. |
| `/checks`, `/files`, `/diff`, `/reviews`, `/mergeable` | **Implemented** | |
| `/request @user` | **Implemented** | |

---

## 7. Commits

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| `/commit <SHA>`, `/commits [branch]`, `/compare <base> <head>` | **Implemented** | SHA validation. |

---

## 8. GitHub Actions

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| `/actions`, `/run`, `/rerun`, `/cancel`, `/logs` | **Implemented** | All audit-logged. |

---

## 9. Releases

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| `/release`, `/release create <tag>`, `/changelog` | **Implemented** | |

---

## 10. Discussions

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| **`/discussion <title>`** | **Newly Added** (was Removed) | Now implemented via GitHub GraphQL `createDiscussion` mutation. Defaults to the first discussion category. Multi-line body supported. |
| **`/discussions`** | **Newly Added** | Lists recent discussions via GraphQL. |
| **`/answered`** | **Newly Added** (was Removed) | Marks a discussion comment as the answer via GraphQL `markDiscussionCommentAsAnswer` mutation. Reply to a discussion comment notification to use. |
| Discussion event formatting | **Implemented** (new) | `DiscussionEvent` and `DiscussionCommentEvent` webhook payloads are now formatted and sent to Telegram. |
| Discussion message context | **Implemented** (new) | Reply-to-GitHub works for discussion comments (stored as `discussion_comment` type). |

> **Note:** Discussions require the repository to have Discussions enabled, and the user's token to have `read:discussion` (for list/view) and `write:discussion` (for create/mark-answer) scopes. If Discussions are not enabled, the GraphQL API returns an error which is surfaced to the user.

---

## 11. Search

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| `/find`, `/pr`, `/search` | **Implemented** | |

---

## 12. Notifications & Webhooks

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| Push events | **Implemented** | |
| Pull request events (all variants) | **Implemented** | |
| Issues + issue comments | **Implemented** | |
| PR reviews + review comments | **Implemented** | |
| Release, fork, star, watch events | **Implemented** | |
| Workflow run, check run, check suite events | **Implemented** | |
| Repository events (rename, etc.) | **Implemented** | Updates stored link name on rename. |
| Create/delete (branch/tag) events | **Implemented** | |
| Member, label, milestone events | **Implemented** | |
| Gollum (wiki), commit comment, status, public events | **Implemented** | |
| Team / membership / organization events | **Implemented** | |
| **Discussion + discussion comment events** | **Newly Added** | Now formatted and sent to Telegram. |
| Webhook signature verification (HMAC-SHA-256) | **Improved** | Constant-time comparison. SHA-1 fallback for legacy configs. |
| **Webhook routing via random route IDs** | **Newly Added** (improvement) | New webhooks use `/webhook/{random_64_char_hex}` instead of `/webhook/{encrypted_chat_id}`. Route IDs are stored in MongoDB, support rotation and revocation, and expose no chat ID in the URL. Backward compatible: old encrypted-token webhooks still work. |
| Reply-to-GitHub (reply to notification → post comment) | **Implemented** | MongoDB-backed `message_contexts` collection. |
| `/mute` (mute forum topic) | **Unsupported** | `go-telegram-bot-api/v5` does not expose `message_thread_id` on incoming messages. Returns a clear "unsupported" message. |
| `/done`, `/read` | **Removed** | Requires GitHub Notifications API. Not implemented. |
| `/activity` | **Removed** | Covered by webhook notifications. |
| **Per-repo, per-event notification settings** | **Newly Added** (was Incomplete) | Full implementation: `/settings` → repo picker → per-event toggle panel with ON/OFF buttons for 16 curated event types. "Enable all" / "Disable all" buttons. Settings stored in MongoDB (`RepoLink.Events`). Webhook `processEvent` now consults these settings before delivering notifications. |
| **`/notifications` command** | **Newly Added** | Alias for `/settings`. |
| Per-repo mute toggle | **Fixed** (was broken) | The `c:mute:` callback button now actually toggles the mute flag (was a no-op before). |
| Back button in settings | **Fixed** (was broken) | `c:cfg:back` now correctly returns to the repo list (was misrouted to `showRepoConfig("back")`). |

---

## 13. Settings & System

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| `/settings`, `/config` | **Implemented** | Now with full per-event toggle UI. |
| **`/notifications`** | **Newly Added** | Alias for `/settings`. |
| `/mute` | **Unsupported** | Library limitation (see above). |
| `/reload` | **Implemented** | Admin-only. |
| `/privacy` | **Implemented** | |
| `/help` | **Implemented** | Updated to reflect new commands and accurate status. |
| `/start` | **Implemented** | |

---

## 14. Security & Infrastructure

| Reference Feature | SWAGGYMUSIC Status | Notes |
|---|---|---|
| AES-GCM encrypted token storage | **Improved** | Strict 32-byte key (AES-256), constant-time tag verification. |
| OAuth state management | **Improved** | MongoDB-backed, single-use, TTL-indexed. |
| Telegram admin permission checks | **Implemented** | Cached for 1 hour, invalidatable via `/reload`. |
| Bot owner configuration | **Improved** | `BOT_OWNER_IDS` env var (comma-separated). |
| Audit logging | **Implemented** (new) | All security-sensitive actions logged. |
| Rate limiting | **Implemented** (new) | Per-user Telegram + GitHub rate limits. |
| Input validation | **Implemented** (new) | Comprehensive validation. |
| SSRF prevention | **Implemented** (new) | GitHub API URL must be HTTPS + allowlisted for Enterprise. |
| Path traversal prevention | **Implemented** (new) | Defense-in-depth at command AND ghops layers. |
| Docker multi-stage build | **Improved** | Non-root runtime user. |
| Docker Compose with MongoDB | **Improved** | Internal network + auth + health checks. |
| `.env.example` with placeholders | **Improved** | All secret values now empty (no fake-looking tokens/passwords). |
| CI workflow | **Implemented** (new) | fmt/vet/build/test/docker build. |
| Graceful shutdown | **Implemented** (new) | |
| Health checks | **Implemented** (new) | |
| **GraphQL client for GitHub** | **Newly Added** | `github.com/shurcooL/graphql` integrated for pin/unpin/draft/ready/discussions. |
| **Webhook route ID store** | **Newly Added** | MongoDB-backed random route IDs with rotation/revocation. |
| **Owner access isolation** | **Verified** | Server-level `GITHUB_TOKEN` is loaded but NEVER used by any command. All GitHub operations use the user's decrypted token. |

---

## 15. Summary of Changes in Latest Update

### Newly Added Features
1. `/updatefile` — update an existing repository file (P1)
2. `/draft` — convert PR to draft via GraphQL (P2)
3. `/ready` — mark draft PR as ready via GraphQL (P3)
4. `/pin` and `/unpin` — pin/unpin issues via GraphQL (P4)
5. `/discussion`, `/discussions`, `/answered` — GitHub Discussions support via GraphQL (P5)
6. Per-event notification settings UI with ON/OFF toggles (P6)
7. `/notifications` command alias (P6)
8. Webhook route ID system with rotation/revocation (P7)
9. Discussion event webhook formatting (P5)

### Fixed Bugs
1. `c:mute:` callback button was a no-op — now actually toggles repo mute (P6)
2. `c:cfg:back` button was misrouted — now correctly returns to repo list (P6)
3. File commands hard-coded `"main"` as branch — now use repo's actual default branch (P1)
4. GitHub Access panel was missing 3 of 8 required buttons — now complete (P9)

### Security Improvements
1. `.env.example` cleaned of all fake-looking secrets (P8)
2. Webhook URLs no longer expose chat IDs (route ID system) (P7)
3. Owner access isolation verified — server token never used by user commands (P11)
4. Full security re-audit completed — all 10 categories passed (P10)

### Documentation Updates
1. `FEATURE_PARITY.md` — this file, updated with all changes
2. `SECURITY_AUDIT.md` — updated with new security posture
3. `TEST_REPORT.md` — new file with test execution results

---

## 16. Remaining Limitations (honestly documented)

### Forum topic targeting
`go-telegram-bot-api/v5` does not expose `message_thread_id` on incoming
messages. This affects:
- `/mute` command (returns "unsupported")
- Webhook delivery to specific forum topics (webhooks deliver to the chat's
  main feed, not a specific topic)

A future migration to a TDLib-based library or a newer fork would resolve
this.

### Encryption key rotation
There is no automated key-rotation script. Operators must manually decrypt
all stored tokens with the old key, re-encrypt with the new key, update
`ENCRYPTION_KEY`, and restart. (Tracked as a known limitation.)

### Server-level GitHub token unused
`GITHUB_TOKEN` is loaded from env but no command currently consumes it. It
is reserved for future owner-only system operations. The startup log
message accurately reflects this: "currently unused by any command —
reserved for future owner-only operations".

### `/done`, `/read`, `/activity`
Removed (not implemented). `/done` and `/read` require the GitHub
Notifications API (separate from repo webhooks). `/activity` is covered by
the webhook notifications.

### GraphQL rate limits
GitHub's GraphQL API has a separate rate limit (5,000 points per hour per
token). Heavy use of `/discussions` listing could exhaust this faster than
the REST rate limit. The bot does not currently track GraphQL rate limit
usage separately.

### Discussion category selection
`/discussion` defaults to the first discussion category returned by GitHub
(usually "General" or "Q&A"). There is no UI to select a category
interactively. Users who need a specific category should create the
discussion via GitHub's web UI.

---

## 17. Honest Disclosure

- The project compiles cleanly with `go build ./...` and `CGO_ENABLED=0`
  static build (Docker-equivalent).
- All unit tests pass with `go test ./...`.
- `go vet ./...` reports no issues.
- `gofmt -l .` reports no formatting issues.
- The bot has NOT been tested against a live Telegram bot token or a live
  GitHub account in this build environment. Operators should test in a
  staging environment before production deployment.
- Docker build was verified syntactically (Dockerfile uses standard
  multi-stage build with `golang:1.23-alpine`). Docker was not available
  in this environment to run an actual `docker build`.
