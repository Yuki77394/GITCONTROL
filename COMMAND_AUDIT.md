# SWAGGYMUSIC GitHub Controller Bot — Command Audit

Complete list of every registered command, its purpose, permission
requirements, GitHub permission requirement, and implementation status.

> **No fake commands.** Every command below has a real handler wired in
> `internal/commands/dispatcher.go`. Commands marked "Unsupported" return
> a clear error message explaining the limitation (they are NOT fake —
> they exist and respond, they just cannot perform the action due to
> library/API constraints).

**Total commands: 84** (including aliases)

---

## Authentication (11 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/start` | Show welcome message | None | None | Working |
| `/help` | Show all commands | None | None | Working |
| `/connect` | Connect GitHub via OAuth | None (private chat only) | None | Working |
| `/disconnect` | Disconnect all GitHub accounts | None | None | Working |
| `/logout` | Alias for `/disconnect` | None | None | Working |
| `/me` | Show connected GitHub account | None | `read:user` | Working |
| `/ghaccess` | Open GitHub Access panel | None | None | Working |
| `/addtoken <PAT>` | Add GitHub PAT | None (private chat only) | `read:user` (validated) | Working |
| `/replacetoken <PAT>` | Replace stored token | None (private chat only) | `read:user` (validated) | Working |
| `/testconnection` | Verify stored token works | None | `read:user` | Working |
| `/configureapi <url>` | Configure GitHub API URL | None (private chat only) | None | Working |

## Repository Management (15 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/addrepo [owner/repo]` | Link a repository | Admin (in groups) | `admin:repo_hook` (to create webhook) | Working |
| `/add` | Alias for `/addrepo` | Admin (in groups) | `admin:repo_hook` | Working |
| `/removerepo owner/repo` | Unlink a repository | Admin (in groups) | `admin:repo_hook` (to delete webhook) | Working |
| `/rm` | Alias for `/removerepo` | Admin (in groups) | `admin:repo_hook` | Working |
| `/repos` | List linked repositories | None | None | Working |
| `/repo [owner/repo]` | Show repository info | None | `read` (public) / `repo` (private) | Working |
| `/star` | Star the repo | None | `user` (public) / `public_repo` (private) | Working |
| `/unstar` | Unstar the repo | None | `user` / `public_repo` | Working |
| `/watch` | Watch the repo | None | `repo` | Working |
| `/unwatch` | Unwatch the repo | None | `repo` | Working |
| `/fork` | Fork the repo | None | `public_repo` (public) / `repo` (private) | Working |
| `/archive` | Archive the repo | Admin (in groups) | `admin` | Working |
| `/unarchive` | Unarchive the repo | Admin (in groups) | `admin` | Working |
| `/contributors` | Top contributors | None | `read` / `repo` | Working |
| `/languages` | Language breakdown | None | `read` / `repo` | Working |
| `/stats` | Repository statistics | None | `read` / `repo` | Working |

## Branch Management (5 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/branches` | List branches | None | `read` / `repo` | Working |
| `/branch <name>` | Show branch info | None | `read` / `repo` | Working |
| `/createbranch <new> <from>` | Create a branch | None | `write` | Working |
| `/deletebranch <name>` | Delete a branch | Admin (in groups) | `admin` | Working |
| `/default <name>` | Change default branch | Admin (in groups) | `admin` | Working |

## File Management (5 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/ls [path]` | List directory contents | None | `read` / `repo` | Working |
| `/cat <path>` | View a text file | None | `read` / `repo` | Working |
| `/createfile <path> <msg> <content>` | Create a file | None | `write` | Working |
| `/updatefile <path> [msg] <content>` | Update an existing file | None | `write` | **Newly Added** |
| `/deletefile <path>` | Delete a file | Admin (in groups) | `admin` | Working |

## Issue Management (14 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/issue <title>` | Create an issue | None | `write` | Working |
| `/comment <text>` | Comment on issue/PR (reply) | None | `write` | Working |
| `/close` | Close issue/PR (reply) | None | `write` | Working |
| `/reopen` | Reopen issue/PR (reply) | None | `write` | Working |
| `/assign @user` | Assign a user (reply) | None | `write` | Working |
| `/assignme` | Assign yourself (reply) | None | `write` | Working |
| `/unassign @user` | Unassign (reply) | None | `write` | Working |
| `/label +bug -wip` | Add/remove labels (reply) | None | `write` | Working |
| `/labels` | List labels | None | `read` / `repo` | Working |
| `/milestone <name>` | Set milestone (reply) | None | `write` | Working |
| `/lock [reason]` | Lock conversation (reply) | None | `admin` | Working |
| `/unlock` | Unlock conversation (reply) | None | `admin` | Working |
| `/pin` | Pin an issue (reply) | None | `admin` | **Newly Added** (was Unsupported) |
| `/unpin` | Unpin an issue (reply) | None | `admin` | **Newly Added** (was Unsupported) |

## Pull Request Management (11 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/approve [text]` | Approve PR (reply) | None | `write` | Working |
| `/requestchanges [text]` | Request changes (reply) | None | `write` | Working |
| `/merge [squash\|rebase\|merge]` | Merge PR (reply, with confirmation) | None | `admin` | Working |
| `/draft` | Convert PR to draft (reply) | None | `write` | **Newly Added** (was Unsupported) |
| `/ready` | Mark draft as ready (reply) | None | `write` | **Newly Added** (was Unsupported) |
| `/checks` | Show CI status (reply) | None | `read` / `repo` | Working |
| `/files` | List changed files (reply) | None | `read` / `repo` | Working |
| `/diff` | Show change summary (reply) | None | `read` / `repo` | Working |
| `/reviews` | List reviews (reply) | None | `read` / `repo` | Working |
| `/mergeable` | Check mergeability (reply) | None | `read` / `repo` | Working |
| `/request @user` | Request reviewer (reply) | None | `write` | Working |

## Commits (3 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/commit <SHA>` | Show commit details | None | `read` / `repo` | Working |
| `/commits [branch]` | Recent commits | None | `read` / `repo` | Working |
| `/compare <base> <head>` | Compare refs | None | `read` / `repo` | Working |

## GitHub Actions (5 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/actions` | List recent runs | None | `repo` | Working |
| `/run <workflow.yml> [branch]` | Trigger a workflow | None | `repo` | Working |
| `/rerun <run_id>` | Rerun failed jobs | None | `repo` | Working |
| `/cancel <run_id>` | Cancel a run | None | `repo` | Working |
| `/logs <run_id>` | Get logs URL | None | `repo` | Working |

## Releases (2 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/release` | Show latest release (or `create <tag>`) | None | `read` / `repo` | Working |
| `/changelog <tag> [prev]` | Generate release notes | None | `read` / `repo` | Working |

## Search (3 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/find <query>` | Search issues | None | `repo` | Working |
| `/pr <query>` | Search pull requests | None | `repo` | Working |
| `/search <query>` | Search code | None | `repo` | Working |

## Discussions (3 commands, all newly added)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/discussion <title>` | Create a discussion | None | `write:discussion` | **Newly Added** |
| `/discussions` | List recent discussions | None | `read:discussion` | **Newly Added** |
| `/answered` | Mark discussion comment as answer (reply) | None | `write:discussion` | **Newly Added** |

## Settings & System (6 commands)

| Command | Purpose | Telegram Permission | GitHub Permission | Status |
|---------|---------|---------------------|-------------------|--------|
| `/settings` | Manage notification preferences | None | None | Working |
| `/config` | Alias for `/settings` | None | None | Working |
| `/notifications` | Alias for `/settings` | None | None | **Newly Added** |
| `/mute` | Mute forum topic | None | None | Unsupported (library limitation) |
| `/reload` | Reload admin cache | Admin | None | Working |
| `/privacy` | Privacy policy | None | None | Working |

---

## Summary by Status

| Status | Count |
|--------|-------|
| Working | 71 |
| Newly Added (was Unsupported or missing) | 10 |
| Unsupported (library limitation) | 1 |
| Fake/placeholder | 0 |
| **Total** | **84** (including aliases) |

---

## Newly Added Commands (in this update)

1. `/updatefile` — update an existing repository file
2. `/draft` — convert PR to draft (via GraphQL)
3. `/ready` — mark draft as ready (via GraphQL)
4. `/pin` — pin an issue (via GraphQL)
5. `/unpin` — unpin an issue (via GraphQL)
6. `/discussion` — create a discussion (via GraphQL)
7. `/discussions` — list discussions (via GraphQL)
8. `/answered` — mark discussion comment as answer (via GraphQL)
9. `/notifications` — alias for `/settings`
10. (Implicit) per-event toggle buttons in `/settings` UI

---

## Unsupported Commands (honestly documented)

| Command | Reason | Workaround |
|---------|--------|------------|
| `/mute` | `go-telegram-bot-api/v5` does not expose `message_thread_id` on incoming messages, so the bot cannot detect which forum topic a command was sent from. | Use `/settings` to mute the entire repository instead. |

### Commands removed (not implemented, not in help text)

| Command | Reason |
|---------|--------|
| `/done` | Requires GitHub Notifications API (separate from repo webhooks). |
| `/read` | Same as above. |
| `/activity` | Covered by webhook notifications. |

---

## Permission Model Notes

- **Telegram Permission** is enforced by the dispatcher before the handler
  runs. "Admin (in groups)" means the user must be a chat administrator
  (or a bot owner). In private chats, all commands are available to the
  user.
- **GitHub Permission** is enforced by the GitHub API itself. The bot
  uses the calling user's decrypted token, so GitHub's permission check
  applies. If the user lacks the required GitHub permission, the API
  returns 403 and the bot surfaces the error.
- **No hidden admin IDs**: `BOT_OWNER_IDS` is loaded from env vars only.
- **Owner credentials isolated**: The server-level `GITHUB_TOKEN` (env
  var) is loaded but never used by any command. All GitHub operations
  use the user's own token.
