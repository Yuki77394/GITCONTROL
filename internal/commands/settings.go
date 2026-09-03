package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/ghops"
	"github.com/swaggymusic/github-bot/internal/github"
	"github.com/swaggymusic/github-bot/internal/telegram"
	"github.com/swaggymusic/github-bot/internal/validation"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// registerSettings registers settings/misc commands.
func (d *Dispatcher) registerSettings() {
	d.Register("settings", Handler{Run: d.cmdSettings, HelpText: "Manage notification preferences."})
	d.Register("config", Handler{Run: d.cmdSettings, HelpText: "Alias for /settings."})
	d.Register("notifications", Handler{Run: d.cmdSettings, HelpText: "Alias for /settings (notification preferences)."})
	d.Register("mute", Handler{Run: d.cmdMute, HelpText: "Mute the current forum topic."})
	d.Register("reload", Handler{Run: d.cmdReload, HelpText: "Reload admin cache (admins).", AdminOnly: true})
	d.Register("privacy", Handler{Run: d.cmdPrivacy, HelpText: "Privacy policy."})
}

// registerMisc registers miscellaneous commands.
func (d *Dispatcher) registerMisc() {
	// (privacy and reload are in registerSettings for grouping)
}

func (d *Dispatcher) cmdSettings(ctx context.Context, m *tgbotapi.Message, args []string) error {
	links, err := d.deps.DB.GetChatLinks(ctx, m.Chat.ID)
	if err != nil || len(links) == 0 {
		_, _ = d.replyMsg(m, "No repositories linked. Use /addrepo first.")
		return nil
	}
	var rows [][]telegram.Button
	for _, l := range links {
		rows = append(rows, []telegram.Button{{Text: l.RepoFullName, Data: "c:cfg:" + l.RepoFullName}})
	}
	markup := telegram.InlineKeyboard(rows)
	_, _ = d.deps.Bot.SendMessage(m.Chat.ID, "<b>Repository settings — select a repo:</b>", "HTML", int32(m.MessageID), 0, markup)
	return nil
}

func (d *Dispatcher) cmdMute(ctx context.Context, m *tgbotapi.Message, args []string) error {
	// Note: tgbotapi v5 does not expose MessageThreadID on Message.
	// Forum topic muting via /mute is therefore not supported in this build.
	// To mute a topic, ask the bot owner to remove the repo link or filter
	// events via /settings.
	_, _ = d.replyMsg(m, "⚠️ /mute is not supported in this build (tgbotapi v5 does not expose MessageThreadID). Use /settings to mute the repository instead.")
	return nil
}

func (d *Dispatcher) cmdReload(ctx context.Context, m *tgbotapi.Message, args []string) error {
	d.deps.Perms.InvalidateAllAdminCache()
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "system.reload", "admin_cache", audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, "✅ Admin cache reloaded.")
	return nil
}

func (d *Dispatcher) cmdPrivacy(ctx context.Context, m *tgbotapi.Message, args []string) error {
	msg := `<b>SWAGGYMUSIC GitHub Controller Bot — Privacy Policy</b>

<b>Data we store:</b>
• Your Telegram user ID, username, and chat metadata.
• Your GitHub user ID, username, and the <b>encrypted</b> OAuth/PAT token.
• The mapping between Telegram chats and linked GitHub repositories.
• Audit log entries (action, target, result, timestamp) — never secrets.

<b>Data we do NOT store:</b>
• Plaintext tokens (only AES-256-GCM encrypted ciphertext).
• Encryption keys (loaded from environment on startup).
• Webhook secrets (loaded from environment on startup).
• Personal message contents beyond the audit log.

<b>How we use your data:</b>
• To authenticate GitHub API calls on your behalf.
• To route GitHub webhook events to the chats you configured.
• To forward your Telegram replies as GitHub comments.

<b>How to remove your data:</b>
• Use /disconnect to delete all your GitHub accounts and encrypted tokens.
• Ask the bot owner to remove chat-level repository links.

<b>Third parties:</b>
• Telegram (message delivery).
• GitHub (API access).
• MongoDB (database, self-hosted).
No analytics, telemetry, or advertising SDKs are used.`
	_, err := d.deps.Bot.SendMessage(m.Chat.ID, msg, "HTML", int32(m.MessageID), 0, nil)
	return err
}

// ---------------------------------------------------------------------------
// Callback handlers
// ---------------------------------------------------------------------------

// dispatchSettingsCallback handles "c:" prefixed callbacks.
//
// Callback data formats:
//
//	c:noop                              — no-op (just answers the callback)
//	c:ar:pg:<page>                      — paginate repo list
//	c:ar:<owner/repo>                   — add repo
//	c:cfg:<owner/repo>                  — show repo config panel
//	c:cfg:back                          — go back to repo list
//	c:mute:<owner/repo>                 — toggle repo mute
//	c:ev:<owner/repo>:<event_name>      — toggle individual event
//	c:evall:<owner/repo>:on             — enable all events
//	c:evall:<owner/repo>:off            — disable all events
//	c:evback:<owner/repo>               — back to repo config from events
//	c:evlist:<owner/repo>               — show events list (alias for cfg)
func (d *Dispatcher) dispatchSettingsCallback(ctx context.Context, cq *tgbotapi.CallbackQuery, data string) {
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return
	}
	switch parts[1] {
	case "noop":
		_ = d.deps.Bot.AnswerCallback(cq.ID, "", false)
	case "ar": // add repo
		if len(parts) < 3 {
			return
		}
		if parts[2] == "pg" {
			if len(parts) < 4 {
				return
			}
			var page int
			fmt.Sscanf(parts[3], "%d", &page)
			if page < 1 {
				page = 1
			}
			_ = d.deps.Bot.AnswerCallback(cq.ID, "", false)
			_ = d.sendRepoList(ctx, cq.Message, page)
			return
		}
		// parts[2] is owner/repo.
		full := strings.Join(parts[2:], ":")
		full = strings.TrimPrefix(full, "//")
		owner, repo, err := splitRepo(full)
		if err != nil {
			_ = d.deps.Bot.AnswerCallback(cq.ID, "Invalid repo", true)
			return
		}
		_ = d.addRepoFromCallback(ctx, cq, owner, repo)
	case "cfg":
		// c:cfg:back → go back to repo list
		if len(parts) >= 3 && parts[2] == "back" {
			_ = d.deps.Bot.AnswerCallback(cq.ID, "", false)
			_ = d.sendSettingsRepoList(ctx, cq.Message)
			return
		}
		if len(parts) < 3 {
			return
		}
		d.showRepoConfig(ctx, cq, strings.Join(parts[2:], ":"))
	case "mute":
		// c:mute:<owner/repo>
		if len(parts) < 3 {
			_ = d.deps.Bot.AnswerCallback(cq.ID, "Missing repo", true)
			return
		}
		repoFullName := strings.Join(parts[2:], ":")
		newMuted, err := d.deps.DB.ToggleRepoLinkMuted(ctx, cq.Message.Chat.ID, repoFullName)
		if err != nil {
			_ = d.deps.Bot.AnswerCallback(cq.ID, fmt.Sprintf("❌ %v", err), true)
			return
		}
		verb := "unmuted"
		if newMuted {
			verb = "muted"
		}
		_ = d.deps.Bot.AnswerCallback(cq.ID, fmt.Sprintf("✅ %s %s", repoFullName, verb), false)
		// Refresh the panel.
		d.showRepoConfig(ctx, cq, repoFullName)
	case "ev":
		// c:ev:<owner/repo>:<event_name>
		if len(parts) < 4 {
			_ = d.deps.Bot.AnswerCallback(cq.ID, "Missing event", true)
			return
		}
		repoFullName := parts[2]
		eventName := strings.Join(parts[3:], ":")
		enabled, err := d.deps.DB.ToggleRepoLinkEvent(ctx, cq.Message.Chat.ID, repoFullName, eventName)
		if err != nil {
			_ = d.deps.Bot.AnswerCallback(cq.ID, fmt.Sprintf("❌ %v", err), true)
			return
		}
		verb := "disabled"
		if enabled {
			verb = "enabled"
		}
		_ = d.deps.Bot.AnswerCallback(cq.ID, fmt.Sprintf("✅ %s %s", eventName, verb), false)
		// Refresh the events panel.
		d.showEventsConfig(ctx, cq, repoFullName)
	case "evall":
		// c:evall:<owner/repo>:on | c:evall:<owner/repo>:off
		if len(parts) < 4 {
			_ = d.deps.Bot.AnswerCallback(cq.ID, "Missing args", true)
			return
		}
		repoFullName := parts[2]
		enable := parts[3] == "on"
		allEvents := allDefaultEventNames()
		if err := d.deps.DB.SetRepoLinkEventsAll(ctx, cq.Message.Chat.ID, repoFullName, allEvents, enable); err != nil {
			_ = d.deps.Bot.AnswerCallback(cq.ID, fmt.Sprintf("❌ %v", err), true)
			return
		}
		verb := "disabled all events"
		if enable {
			verb = "enabled all events"
		}
		_ = d.deps.Bot.AnswerCallback(cq.ID, fmt.Sprintf("✅ %s: %s", repoFullName, verb), false)
		d.showEventsConfig(ctx, cq, repoFullName)
	case "evback":
		// c:evback:<owner/repo> → back to repo config
		if len(parts) < 3 {
			return
		}
		_ = d.deps.Bot.AnswerCallback(cq.ID, "", false)
		d.showRepoConfig(ctx, cq, strings.Join(parts[2:], ":"))
	case "evlist":
		// c:evlist:<owner/repo> → show per-event settings panel
		if len(parts) < 3 {
			return
		}
		_ = d.deps.Bot.AnswerCallback(cq.ID, "", false)
		d.showEventsConfig(ctx, cq, strings.Join(parts[2:], ":"))
	}
}

// sendSettingsRepoList shows the top-level repo picker for /settings.
// Used when returning "back" from a per-repo config panel.
func (d *Dispatcher) sendSettingsRepoList(ctx context.Context, m *tgbotapi.Message) error {
	links, err := d.deps.DB.GetChatLinks(ctx, m.Chat.ID)
	if err != nil || len(links) == 0 {
		_, _ = d.replyMsg(m, "No repositories linked. Use /addrepo first.")
		return nil
	}
	var rows [][]telegram.Button
	for _, l := range links {
		rows = append(rows, []telegram.Button{{Text: l.RepoFullName, Data: "c:cfg:" + l.RepoFullName}})
	}
	markup := telegram.InlineKeyboard(rows)
	_, _ = d.deps.Bot.SendMessage(m.Chat.ID, "<b>Repository settings — select a repo:</b>", "HTML", int32(m.MessageID), 0, markup)
	return nil
}

func (d *Dispatcher) showRepoConfig(ctx context.Context, cq *tgbotapi.CallbackQuery, repoFullName string) {
	link, err := d.deps.DB.GetRepoLink(ctx, cq.Message.Chat.ID, repoFullName)
	if err != nil {
		_ = d.deps.Bot.AnswerCallback(cq.ID, "Repo not found", true)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Config: %s</b>\n", repoFullName)
	fmt.Fprintf(&b, "Muted: %v\n", link.Muted)
	fmt.Fprintf(&b, "Events: %d enabled\n", len(link.Events))
	rows := [][]telegram.Button{
		{{Text: toggleMuteLabel(link.Muted), Data: "c:mute:" + repoFullName}},
		{{Text: "🔔 Per-event settings", Data: "c:evlist:" + repoFullName}},
		{{Text: "◀️ Back", Data: "c:cfg:back"}},
	}
	markup := telegram.InlineKeyboard(rows)
	_ = d.deps.Bot.EditText(cq.Message.Chat.ID, int64(cq.Message.MessageID), b.String(), "HTML", markup)
	_ = d.deps.Bot.AnswerCallback(cq.ID, "", false)
}

// showEventsConfig renders the per-event toggle panel for a repo.
// Shows each supported event with an ON/OFF button.
func (d *Dispatcher) showEventsConfig(ctx context.Context, cq *tgbotapi.CallbackQuery, repoFullName string) {
	link, err := d.deps.DB.GetRepoLink(ctx, cq.Message.Chat.ID, repoFullName)
	if err != nil {
		_ = d.deps.Bot.AnswerCallback(cq.ID, "Repo not found", true)
		return
	}
	enabledSet := make(map[string]bool, len(link.Events))
	for _, e := range link.Events {
		enabledSet[e] = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>🔔 Notification Settings</b>\n")
	fmt.Fprintf(&b, "<b>Repo:</b> %s\n\n", repoFullName)
	// Show the curated list of user-facing events (not all 43 — too many).
	displayEvents := userFacingEvents()
	for _, ev := range displayEvents {
		status := "OFF"
		if enabledSet[ev.Name] {
			status = "ON"
		}
		fmt.Fprintf(&b, "%s: %s\n", ev.Label, status)
	}
	// Build the toggle buttons (2 per row to fit Telegram's width).
	var rows [][]telegram.Button
	for i := 0; i < len(displayEvents); i += 2 {
		var row []telegram.Button
		ev := displayEvents[i]
		row = append(row, telegram.Button{
			Text: fmt.Sprintf("%s: %s", shortLabel(ev.Label), onOffLabel(enabledSet[ev.Name])),
			Data: "c:ev:" + repoFullName + ":" + ev.Name,
		})
		if i+1 < len(displayEvents) {
			ev2 := displayEvents[i+1]
			row = append(row, telegram.Button{
				Text: fmt.Sprintf("%s: %s", shortLabel(ev2.Label), onOffLabel(enabledSet[ev2.Name])),
				Data: "c:ev:" + repoFullName + ":" + ev2.Name,
			})
		}
		rows = append(rows, row)
	}
	// Enable/disable all + back.
	rows = append(rows, []telegram.Button{
		{Text: "✅ Enable all", Data: "c:evall:" + repoFullName + ":on"},
		{Text: "❌ Disable all", Data: "c:evall:" + repoFullName + ":off"},
	})
	rows = append(rows, []telegram.Button{
		{Text: "◀️ Back to repo config", Data: "c:evback:" + repoFullName},
	})
	markup := telegram.InlineKeyboard(rows)
	_ = d.deps.Bot.EditText(cq.Message.Chat.ID, int64(cq.Message.MessageID), b.String(), "HTML", markup)
	_ = d.deps.Bot.AnswerCallback(cq.ID, "", false)
}

func toggleMuteLabel(muted bool) string {
	if muted {
		return "🔔 Unmute"
	}
	return "🔕 Mute"
}

func onOffLabel(on bool) string {
	if on {
		return "ON"
	}
	return "OFF"
}

// shortLabel truncates a label to ~10 chars for inline keyboard buttons.
func shortLabel(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:10] + "…"
}

// userFacingEvents is the curated subset of GitHub events that are exposed
// in the per-event settings UI. We don't show all 43 supported events
// because the Telegram inline keyboard would be too large.
func userFacingEvents() []github.Event {
	return []github.Event{
		{Name: "push", Label: "Push"},
		{Name: "pull_request", Label: "Pull Requests"},
		{Name: "issues", Label: "Issues"},
		{Name: "issue_comment", Label: "Issue Comments"},
		{Name: "pull_request_review", Label: "PR Reviews"},
		{Name: "pull_request_review_comment", Label: "PR Review Comments"},
		{Name: "workflow_run", Label: "Workflow Runs"},
		{Name: "release", Label: "Releases"},
		{Name: "fork", Label: "Forks"},
		{Name: "star", Label: "Stars"},
		{Name: "repository", Label: "Repo Changes"},
		{Name: "check_run", Label: "Check Runs"},
		{Name: "create", Label: "Branch/Tag Created"},
		{Name: "delete", Label: "Branch/Tag Deleted"},
		{Name: "discussion", Label: "Discussions"},
		{Name: "discussion_comment", Label: "Discussion Comments"},
	}
}

// allDefaultEventNames returns the full default event list (used by
// "enable all" / "disable all").
func allDefaultEventNames() []string {
	return defaultEvents()
}

func (d *Dispatcher) addRepoFromCallback(ctx context.Context, cq *tgbotapi.CallbackQuery, owner, repo string) error {
	// Reuse the AddRepo logic with synthetic args.
	m := &tgbotapi.Message{
		MessageID: cq.Message.MessageID,
		Chat:      cq.Message.Chat,
		From:      cq.From,
		Text:      "/addrepo " + owner + "/" + repo,
		Entities:  []tgbotapi.MessageEntity{},
	}
	m.Chat.ID = cq.Message.Chat.ID
	// Use cq.From as the user.
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	d.cmdAddRepo(cmdCtx, m, []string{owner + "/" + repo})
	return nil
}

// dispatchActionCallback handles "act:" prefixed callbacks (PR merge confirmations).
func (d *Dispatcher) dispatchActionCallback(ctx context.Context, cq *tgbotapi.CallbackQuery, data string) {
	parts := strings.SplitN(data, ":", 3)
	if len(parts) < 3 {
		return
	}
	action := parts[1]
	key := parts[2]
	ac, ok := d.GetPRActionContext(key)
	if !ok {
		_ = d.deps.Bot.AnswerCallback(cq.ID, "Action expired", true)
		return
	}
	switch action {
	case "merge":
		// Verify GitHub permission by attempting the merge.
		client, _, err := d.deps.Access.GetDecryptedClient(ctx, cq.From.ID)
		if err != nil {
			_ = d.deps.Bot.AnswerCallback(cq.ID, "No GitHub account", true)
			return
		}
		method := ghops.MergeMethod(ac.Method)
		if err := ghops.MergePR(ctx, client, ac.Owner, ac.Repo, ac.PRNumber, method, "", ""); err != nil {
			_ = d.deps.Bot.AnswerCallback(cq.ID, fmt.Sprintf("Merge failed: %v", err), true)
			return
		}
		d.deps.Audit.Log(ctx, cq.From.ID, cq.From.UserName, "pr.merge", fmt.Sprintf("%s/%s#%d", ac.Owner, ac.Repo, ac.PRNumber), audit.ResultSuccess, "method="+ac.Method, cq.Message.Chat.ID)
		_ = d.deps.Bot.AnswerCallback(cq.ID, "✅ Merged", false)
		_, _ = d.deps.Bot.SendHTML(cq.Message.Chat.ID, fmt.Sprintf("✅ PR #%d merged via <code>%s</code> by %s.", ac.PRNumber, ac.Method, cq.From.FirstName))
	case "cancel":
		_ = d.deps.Bot.AnswerCallback(cq.ID, "Cancelled", false)
		_, _ = d.deps.Bot.SendHTML(cq.Message.Chat.ID, "❌ Merge cancelled.")
	}
}

// dispatchAccessCallback handles "gh:" prefixed callbacks (GitHub Access panel).
func (d *Dispatcher) dispatchAccessCallback(ctx context.Context, cq *tgbotapi.CallbackQuery, data string) {
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return
	}
	switch parts[1] {
	case "connect":
		_ = d.deps.Bot.AnswerCallback(cq.ID, "Use /connect in private chat to start OAuth.", true)
	case "addtoken":
		_ = d.deps.Bot.AnswerCallback(cq.ID, "Use /addtoken <PAT> in private chat.", true)
	case "test":
		login, err := d.deps.Access.TestConnection(ctx, cq.From.ID)
		if err != nil {
			_ = d.deps.Bot.AnswerCallback(cq.ID, fmt.Sprintf("❌ %v", err), true)
			return
		}
		_ = d.deps.Bot.AnswerCallback(cq.ID, fmt.Sprintf("✅ Connected as %s", login), false)
	case "replace":
		_ = d.deps.Bot.AnswerCallback(cq.ID, "Use /replacetoken <new PAT> in private chat.", true)
	case "cfgapi":
		_ = d.deps.Bot.AnswerCallback(cq.ID, "Use /configureapi <url> to set the GitHub API URL.", true)
	case "addrepo":
		// Reuse the interactive repo list from /addrepo.
		_ = d.deps.Bot.AnswerCallback(cq.ID, "", false)
		_ = d.sendRepoList(ctx, cq.Message, 1)
	case "selectrepo":
		// Show the chat's linked repos for selection.
		_ = d.deps.Bot.AnswerCallback(cq.ID, "", false)
		links, err := d.deps.DB.GetChatLinks(ctx, cq.Message.Chat.ID)
		if err != nil || len(links) == 0 {
			_, _ = d.deps.Bot.SendHTML(cq.Message.Chat.ID, "No repositories linked. Use /addrepo first.")
			return
		}
		var rows [][]telegram.Button
		for _, l := range links {
			rows = append(rows, []telegram.Button{{Text: l.RepoFullName, Data: "c:cfg:" + l.RepoFullName}})
		}
		markup := telegram.InlineKeyboard(rows)
		_, _ = d.deps.Bot.SendMessage(cq.Message.Chat.ID, "Select a repository:", "HTML", 0, 0, markup)
	case "disconnect":
		if err := d.deps.Access.Disconnect(ctx, cq.From.ID); err != nil {
			_ = d.deps.Bot.AnswerCallback(cq.ID, fmt.Sprintf("❌ %v", err), true)
			return
		}
		_ = d.deps.Bot.AnswerCallback(cq.ID, "✅ Disconnected", false)
	}
}

// splitRepo splits "owner/repo" — re-exported for use in this file.
func splitRepo(s string) (string, string, error) {
	return validation.ValidateRepoName(s)
}

// keep imports alive
var (
	_ = github.SupportedEvents
	_ = time.Second
)
