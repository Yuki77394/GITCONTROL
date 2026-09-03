package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/github"
	"github.com/swaggymusic/github-bot/internal/telegram"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// registerAuth registers authentication commands.
func (d *Dispatcher) registerAuth() {
	d.Register("start", Handler{
		Run:      d.cmdStart,
		HelpText: "Show the welcome message.",
	})
	d.Register("help", Handler{
		Run:      d.cmdHelp,
		HelpText: "Show all available commands.",
	})
	d.Register("connect", Handler{
		Run:         d.cmdConnect,
		HelpText:    "Connect your GitHub account via OAuth (private chat only).",
		PrivateOnly: true,
	})
	d.Register("disconnect", Handler{
		Run:      d.cmdDisconnect,
		HelpText: "Disconnect all your GitHub accounts.",
	})
	d.Register("logout", Handler{
		Run:      d.cmdDisconnect,
		HelpText: "Alias for /disconnect.",
	})
	d.Register("me", Handler{
		Run:      d.cmdMe,
		HelpText: "Show your connected GitHub account.",
	})
	d.Register("ghaccess", Handler{
		Run:      d.cmdGitHubAccess,
		HelpText: "Open the GitHub Access panel.",
	})
	d.Register("addtoken", Handler{
		Run:         d.cmdAddToken,
		HelpText:    "Add or replace a GitHub Personal Access Token (private chat only).",
		PrivateOnly: true,
	})
	d.Register("replacetoken", Handler{
		Run:         d.cmdReplaceToken,
		HelpText:    "Replace a stored GitHub token (private chat only).",
		PrivateOnly: true,
	})
	d.Register("testconnection", Handler{
		Run:      d.cmdTestConnection,
		HelpText: "Test that your stored GitHub token still works.",
	})
	d.Register("configureapi", Handler{
		Run:         d.cmdConfigureAPI,
		HelpText:    "Configure the GitHub API URL (owner-only or enterprise allowlist).",
		OwnerOnly:   false,
		PrivateOnly: true,
	})
}

func (d *Dispatcher) cmdStart(ctx context.Context, m *tgbotapi.Message, args []string) error {
	msg := `<b>SWAGGYMUSIC GitHub Controller Bot</b> 🤖

Manage your GitHub repositories, issues, PRs, actions, releases, and webhooks — directly from Telegram.

<b>Get Started:</b>
1. Use /ghaccess to open the GitHub Access panel.
2. Use /connect (OAuth) or /addtoken (PAT) to link a GitHub account.
3. Use /addrepo to link a repository and start receiving notifications.
4. Use /settings to customise notification preferences.

Use /help for the full command list.`
	_, err := d.deps.Bot.SendMessage(m.Chat.ID, msg, "HTML", int32(m.MessageID), 0, nil)
	return err
}

func (d *Dispatcher) cmdHelp(ctx context.Context, m *tgbotapi.Message, args []string) error {
	msg := `<b>SWAGGYMUSIC GitHub Controller Bot — Commands</b>

<b>Authentication</b>
/connect — Connect GitHub via OAuth (private chat)
/addtoken &lt;token&gt; — Add a GitHub Personal Access Token (private chat)
/replacetoken &lt;new_token&gt; — Replace your stored token
/testconnection — Verify your stored token works
/disconnect — Disconnect all GitHub accounts
/me — Show your connected account
/ghaccess — Open the GitHub Access panel
/configureapi &lt;url&gt; — Configure GitHub API URL (Enterprise)

<b>Repository Management</b>
/addrepo [owner/repo] — Link a repository
/removerepo owner/repo — Unlink a repository
/repos — List linked repositories
/repo [owner/repo] — Show repository info
/star, /unstar — Star / unstar the repo
/watch, /unwatch — Watch / unwatch the repo
/fork — Fork the repo
/archive, /unarchive — Archive / unarchive (admin)
/contributors — Top contributors
/languages — Language breakdown

<b>Branches</b>
/branches — List branches
/branch &lt;name&gt; — Show branch info
/createbranch &lt;new&gt; &lt;from&gt; — Create a branch
/deletebranch &lt;name&gt; — Delete a branch (admin)
/default &lt;name&gt; — Change default branch (admin)

<b>Files</b>
/ls [path] — List directory contents
/cat &lt;path&gt; — View a text file
/createfile &lt;path&gt; &lt;message&gt; &lt;content&gt; — Create a file
/updatefile &lt;path&gt; [commit_message] &lt;content&gt; — Update an existing file
/deletefile &lt;path&gt; — Delete a file (admin)

<b>Issues</b>
/issue &lt;title&gt; — Create an issue (multi-line body supported)
/comment &lt;text&gt; — Comment on a replied issue/PR
/close — Close issue/PR (reply to notification)
/reopen — Reopen issue/PR
/assign @user — Assign a user
/assignme — Assign yourself
/unassign @user — Unassign
/label +bug -wip — Add/remove labels
/labels — List labels
/milestone &lt;name&gt; — Set milestone
/lock [reason] — Lock conversation
/unlock — Unlock conversation
/pin, /unpin — Pin / unpin an issue (via GitHub GraphQL API)

<b>Pull Requests</b>
/approve [text] — Approve PR (reply to notification)
/requestchanges [text] — Request changes
/merge [squash|rebase|merge] — Merge PR
/draft — Convert PR to draft (via GitHub GraphQL API)
/ready — Mark draft as ready for review (via GitHub GraphQL API)
/checks — Show CI status
/files — List changed files
/diff — Show change summary
/reviews — List reviews
/mergeable — Check mergeability
/request @user — Request reviewer

<b>Commits</b>
/commit &lt;SHA&gt; — Show commit details
/commits — Recent commits
/compare &lt;base&gt; &lt;head&gt; — Compare refs

<b>GitHub Actions</b>
/actions — List recent runs
/run &lt;workflow.yml&gt; [branch] — Trigger a workflow
/rerun — Rerun failed jobs (reply to notification)
/cancel — Cancel a run (reply to notification)
/logs — Get a link to run logs

<b>Releases</b>
/release — Latest release
/release create &lt;tag&gt; — Create a release
/changelog [tag] — Generate release notes

<b>Search</b>
/find &lt;query&gt; — Search issues
/pr &lt;query&gt; — Search pull requests
/search &lt;query&gt; — Search code

<b>Discussions</b> (requires Discussions enabled on the repo)
/discussion &lt;title&gt; — Create a discussion (multi-line body supported)
/discussions — List recent discussions
/answered — Mark a discussion comment as the answer (reply to notification)

<b>Settings &amp; System</b>
/settings, /config, /notifications — Manage notification preferences
/mute — Mute the current forum topic (not supported in this build)
/reload — Reload admin cache (admins)
/privacy — Privacy policy
/help — This message
`
	_, err := d.deps.Bot.SendMessage(m.Chat.ID, msg, "HTML", int32(m.MessageID), 0, nil)
	return err
}

func (d *Dispatcher) cmdConnect(ctx context.Context, m *tgbotapi.Message, args []string) error {
	if d.deps.OAuth == nil {
		_, err := d.deps.Bot.SendHTML(m.Chat.ID, "⚠️ GitHub OAuth is not configured on this server. Use /addtoken to add a Personal Access Token instead.")
		return err
	}
	state, err := generateOAuthState()
	if err != nil {
		return err
	}
	// Persist state to DB (single-use, 10-minute expiry).
	if d.deps.OAuthSaver != nil {
		if err := d.deps.OAuthSaver.SaveState(ctx, state, m.From.ID); err != nil {
			return err
		}
	} else {
		d.deps.StateCache.Set(state, m.From.ID, 10*time.Minute)
	}
	url := d.deps.OAuth.GetLoginURL(state)
	msg := fmt.Sprintf("Please [connect your GitHub account](%s) to enable webhook setup and GitHub operations.", url)
	_, err = d.deps.Bot.SendMessage(m.Chat.ID, msg, "Markdown", int32(m.MessageID), 0, nil)
	return err
}

func (d *Dispatcher) cmdDisconnect(ctx context.Context, m *tgbotapi.Message, args []string) error {
	if err := d.deps.Access.Disconnect(ctx, m.From.ID); err != nil {
		return err
	}
	_, err := d.deps.Bot.SendHTML(m.Chat.ID, "✅ All your GitHub accounts have been disconnected.")
	return err
}

func (d *Dispatcher) cmdMe(ctx context.Context, m *tgbotapi.Message, args []string) error {
	st := d.deps.Access.GetStatus(ctx, m.From.ID)
	if !st.Connected {
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, "⚠️ You have not connected a GitHub account. Use /connect or /addtoken.")
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>GitHub Account</b>\n")
	fmt.Fprintf(&b, "Username: <b>%s</b>\n", telegram.EscapeMarkdownV2(st.GitHubUsername))
	fmt.Fprintf(&b, "Auth method: <code>%s</code>\n", st.AuthMethod)
	fmt.Fprintf(&b, "API URL: <code>%s</code>\n", st.APIURL)
	if st.HasMultipleAccts {
		fmt.Fprintf(&b, "Accounts connected: <code>%d</code>\n", st.AccountCount)
	}
	_, err := d.deps.Bot.SendMessage(m.Chat.ID, b.String(), "HTML", int32(m.MessageID), 0, nil)
	return err
}

func (d *Dispatcher) cmdGitHubAccess(ctx context.Context, m *tgbotapi.Message, args []string) error {
	st := d.deps.Access.GetStatus(ctx, m.From.ID)
	var b strings.Builder
	b.WriteString("<b>GitHub Access</b>\n\n")
	if st.Connected {
		b.WriteString("Status: <b>Connected</b>\n")
		b.WriteString(fmt.Sprintf("Account: <b>%s</b>\n", st.GitHubUsername))
		b.WriteString(fmt.Sprintf("Authentication: <code>%s</code>\n", st.AuthMethod))
		b.WriteString(fmt.Sprintf("GitHub API: <code>%s</code>\n", st.APIURL))
		if st.HasMultipleAccts {
			b.WriteString(fmt.Sprintf("Accounts: <code>%d</code>\n", st.AccountCount))
		}
		b.WriteString("Token: <code>Configured</code>\n")
	} else {
		b.WriteString("Status: <b>Not Connected</b>\n")
		b.WriteString(fmt.Sprintf("GitHub API: <code>%s</code>\n", st.APIURL))
		b.WriteString("Token: <code>Not configured</code>\n")
	}
	// Show the active repository (first linked repo for the chat, if any).
	links, _ := d.deps.DB.GetChatLinks(ctx, m.Chat.ID)
	if len(links) > 0 {
		b.WriteString(fmt.Sprintf("Active Repository: <code>%s</code>\n", links[0].RepoFullName))
	} else {
		b.WriteString("Active Repository: <i>none</i>\n")
	}
	// Inline keyboard — 8 buttons required by the spec.
	rows := [][]telegram.Button{
		{telegram.Button{Text: "🔌 Connect GitHub", Data: "gh:connect"}},
		{telegram.Button{Text: "🔑 Add Access Token", Data: "gh:addtoken"}},
		{telegram.Button{Text: "🔄 Replace Token", Data: "gh:replace"}},
		{telegram.Button{Text: "🧪 Test Connection", Data: "gh:test"}},
		{telegram.Button{Text: "🌐 Configure API URL", Data: "gh:cfgapi"}},
		{telegram.Button{Text: "➕ Add Repository", Data: "gh:addrepo"}},
		{telegram.Button{Text: "📋 Select Repository", Data: "gh:selectrepo"}},
		{telegram.Button{Text: "❌ Disconnect", Data: "gh:disconnect"}},
	}
	markup := telegram.InlineKeyboard(rows)
	_, err := d.deps.Bot.SendMessage(m.Chat.ID, b.String(), "HTML", int32(m.MessageID), 0, markup)
	return err
}

func (d *Dispatcher) cmdAddToken(ctx context.Context, m *tgbotapi.Message, args []string) error {
	if len(args) < 1 {
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, "Usage: <code>/addtoken &lt;github_pat&gt;</code>\n\nYour token will be validated, encrypted, and stored. It will never be displayed again.")
		// Delete the user's message containing the token after we reply.
		if m.MessageID != 0 {
			_ = d.deps.Bot.DeleteMessage(m.Chat.ID, int64(m.MessageID))
		}
		return nil
	}
	token := strings.Join(args, " ")
	// Best effort: delete the user's message so the token doesn't linger in chat.
	_ = d.deps.Bot.DeleteMessage(m.Chat.ID, int64(m.MessageID))
	username, err := d.deps.Access.StorePAT(ctx, m.From.ID, token, d.deps.Cfg.GitHubAPIURL)
	if err != nil {
		d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "ghaccess.addtoken", "self", audit.ResultFailure, err.Error(), m.Chat.ID)
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, fmt.Sprintf("❌ Token validation failed: <code>%v</code>", err))
		return nil
	}
	_, err = d.deps.Bot.SendHTML(m.Chat.ID, fmt.Sprintf("✅ GitHub account <b>%s</b> connected via PAT. Your token message has been deleted for safety.", username))
	return err
}

func (d *Dispatcher) cmdReplaceToken(ctx context.Context, m *tgbotapi.Message, args []string) error {
	if len(args) < 1 {
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, "Usage: <code>/replacetoken &lt;new_pat&gt;</code>")
		return nil
	}
	token := strings.Join(args, " ")
	_ = d.deps.Bot.DeleteMessage(m.Chat.ID, int64(m.MessageID))
	// Get the user's default account.
	acc, err := d.deps.DB.GetGitHubAccount(ctx, m.From.ID)
	if err != nil {
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, "❌ No GitHub account to replace. Use /addtoken first.")
		return nil
	}
	if err := d.deps.Access.ReplaceToken(ctx, m.From.ID, acc.GitHubUserID, token, acc.APIURL); err != nil {
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, fmt.Sprintf("❌ Replace failed: <code>%v</code>", err))
		return nil
	}
	_, err = d.deps.Bot.SendHTML(m.Chat.ID, "✅ Token replaced successfully.")
	return err
}

func (d *Dispatcher) cmdTestConnection(ctx context.Context, m *tgbotapi.Message, args []string) error {
	login, err := d.deps.Access.TestConnection(ctx, m.From.ID)
	if err != nil {
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, fmt.Sprintf("❌ Connection test failed: <code>%v</code>", err))
		return nil
	}
	_, err = d.deps.Bot.SendHTML(m.Chat.ID, fmt.Sprintf("✅ Connection OK. Authenticated as <b>%s</b>.", login))
	return err
}

func (d *Dispatcher) cmdConfigureAPI(ctx context.Context, m *tgbotapi.Message, args []string) error {
	if len(args) < 1 {
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, fmt.Sprintf("Current GitHub API URL: <code>%s</code>\n\nUsage: <code>/configureapi &lt;url&gt;</code>\n\nTo set an Enterprise URL, the host must be in the server's GITHUB_ENTERPRISE_ALLOWLIST.", d.deps.Cfg.GitHubAPIURL))
		return nil
	}
	raw := strings.Join(args, " ")
	url, err := d.deps.Access.ConfigureAPIURL(raw, d.deps.Cfg.GitHubEnterpriseAllow)
	if err != nil {
		_, _ = d.deps.Bot.SendHTML(m.Chat.ID, fmt.Sprintf("❌ Invalid API URL: <code>%v</code>", err))
		return nil
	}
	_, err = d.deps.Bot.SendHTML(m.Chat.ID, fmt.Sprintf("✅ Validated GitHub API URL: <code>%s</code>\n\nNote: per-account API URL is set automatically when you connect with /connect or /addtoken.", url))
	return err
}

// generateOAuthState delegates to the github package.
func generateOAuthState() (string, error) {
	return github.GenerateState()
}

// replyMsg is a small helper for command handlers to send a reply.
func (d *Dispatcher) replyMsg(m *tgbotapi.Message, text string) (int64, error) {
	return d.deps.Bot.SendMessage(m.Chat.ID, text, "HTML", int32(m.MessageID), 0, nil)
}
