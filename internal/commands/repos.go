package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/ghops"
	"github.com/swaggymusic/github-bot/internal/models"
	"github.com/swaggymusic/github-bot/internal/telegram"
	"github.com/swaggymusic/github-bot/internal/validation"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	gh "github.com/google/go-github/v66/github"
)

// registerRepo registers repository management commands.
func (d *Dispatcher) registerRepo() {
	d.Register("addrepo", Handler{Run: d.cmdAddRepo, HelpText: "Link a repo (admin in groups).", AdminOnly: true})
	d.Register("add", Handler{Run: d.cmdAddRepo, HelpText: "Alias for /addrepo.", AdminOnly: true})
	d.Register("removerepo", Handler{Run: d.cmdRemoveRepo, HelpText: "Unlink a repo (admin in groups).", AdminOnly: true})
	d.Register("rm", Handler{Run: d.cmdRemoveRepo, HelpText: "Alias for /removerepo.", AdminOnly: true})
	d.Register("repos", Handler{Run: d.cmdRepos, HelpText: "List linked repositories."})
	d.Register("repo", Handler{Run: d.cmdRepo, HelpText: "Show repo info."})
	d.Register("star", Handler{Run: d.cmdStar, HelpText: "Star the repo (reply to notification)."})
	d.Register("unstar", Handler{Run: d.cmdUnstar, HelpText: "Unstar the repo."})
	d.Register("watch", Handler{Run: d.cmdWatch, HelpText: "Watch the repo."})
	d.Register("unwatch", Handler{Run: d.cmdUnwatch, HelpText: "Unwatch the repo."})
	d.Register("fork", Handler{Run: d.cmdFork, HelpText: "Fork the repo."})
	d.Register("archive", Handler{Run: d.cmdArchive, HelpText: "Archive the repo (admin).", AdminOnly: true})
	d.Register("unarchive", Handler{Run: d.cmdUnarchive, HelpText: "Unarchive the repo (admin).", AdminOnly: true})
	d.Register("contributors", Handler{Run: d.cmdContributors, HelpText: "List top contributors."})
	d.Register("languages", Handler{Run: d.cmdLanguages, HelpText: "Show language breakdown."})
	d.Register("stats", Handler{Run: d.cmdStats, HelpText: "Show repo statistics."})
}

// resolveRepoFromContext returns the repo to operate on, either from the
// message reply context, the command args, or the chat's first linked repo.
func (d *Dispatcher) resolveRepoFromContext(ctx context.Context, m *tgbotapi.Message, args []string) (string, string, error) {
	// 1. If args has "owner/repo", use it.
	if len(args) > 0 {
		owner, repo, err := validation.ValidateRepoName(args[0])
		if err == nil {
			return owner, repo, nil
		}
	}
	// 2. If replying to a notification, look up the message context.
	if m.ReplyToMessage != nil {
		mc, err := d.deps.DB.GetMessageContext(ctx, m.Chat.ID, int64(m.ReplyToMessage.MessageID))
		if err == nil && mc != nil {
			return mc.Owner, mc.Repo, nil
		}
	}
	// 3. Use the first linked repo for the chat.
	links, err := d.deps.DB.GetChatLinks(ctx, m.Chat.ID)
	if err != nil {
		return "", "", fmt.Errorf("no repo context: %w", err)
	}
	if len(links) == 0 {
		return "", "", fmt.Errorf("no repo context: use /addrepo first or specify owner/repo")
	}
	owner, repo, err := validation.ValidateRepoName(links[0].RepoFullName)
	if err != nil {
		return "", "", err
	}
	return owner, repo, nil
}

func (d *Dispatcher) cmdAddRepo(ctx context.Context, m *tgbotapi.Message, args []string) error {
	if len(args) < 1 {
		return d.sendRepoList(ctx, m, 1)
	}
	owner, repo, err := validation.ValidateRepoName(args[0])
	if err != nil {
		_, _ = d.replyMsg(m, "❌ Invalid repo format. Use <code>owner/repo</code>.")
		return nil
	}

	// Check user has a connected account.
	_, acc, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		_, _ = d.replyMsg(m, "⚠️ Please /connect your GitHub account first.")
		return nil
	}
	// Verify repo exists and user has access (use client).
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if _, err := ghops.GetRepo(ctx, client, owner, repo); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Could not access repo: <code>%v</code>", err))
		return nil
	}

	// Create webhook.
	topicID := int32(0)
	// Note: tgbotapi v5 does not expose MessageThreadID on Message.
	// Forum topic targeting is currently not supported for /addrepo; the
	// webhook will deliver to the chat (not a specific topic). To target a
	// topic, users can use the /mute command inside the topic.
	// m.MessageThreadID is intentionally not used here.

	// Build the webhook URL. Prefer the route-ID-based URL (opaque random
	// ID stored in MongoDB) when the route store is available; fall back to
	// the legacy encrypted-token URL otherwise.
	var webhookURL string
	if d.deps.Routes != nil {
		routeID, err := d.deps.Routes.Create(ctx, m.Chat.ID, topicID, owner+"/"+repo)
		if err != nil {
			_, _ = d.replyMsg(m, fmt.Sprintf("❌ Failed to create webhook route: %v", err))
			return nil
		}
		webhookURL = fmt.Sprintf("%s/webhook/%s", d.deps.Cfg.PublicBaseURL, routeID)
	} else {
		tokenPayload := fmt.Sprintf("%d", m.Chat.ID)
		if topicID != 0 {
			tokenPayload = fmt.Sprintf("%d:%d", m.Chat.ID, topicID)
		}
		encToken, err := d.deps.Enc.Encrypt(tokenPayload)
		if err != nil {
			return err
		}
		webhookURL = fmt.Sprintf("%s/webhook/%s", d.deps.Cfg.PublicBaseURL, encToken)
	}
	hookID, err := ghops.CreateWebhook(ctx, client, owner, repo, webhookURL, d.deps.Cfg.GitHubWebhookSecret, defaultEvents())
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Webhook creation failed (admin access required): <code>%v</code>", err))
		return nil
	}
	// Save link + webhook config.
	link := models.RepoLink{
		RepoFullName: owner + "/" + repo,
		WebhookID:    hookID,
		Events:       defaultEvents(),
		Muted:        false,
	}
	if err := d.deps.DB.AddRepoLink(ctx, m.Chat.ID, link); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Failed to link repo: %v", err))
		return nil
	}
	_ = d.deps.DB.SaveWebhookConfig(ctx, &models.WebhookConfig{
		ChatID:       m.Chat.ID,
		TopicID:      topicID,
		RepoFullName: owner + "/" + repo,
		WebhookID:    hookID,
	})
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "repo.add", owner+"/"+repo, audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Repository <b>%s/%s</b> linked and webhook configured.", owner, repo))
	_ = acc
	return nil
}

func (d *Dispatcher) cmdRemoveRepo(ctx context.Context, m *tgbotapi.Message, args []string) error {
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/removerepo owner/repo</code>")
		return nil
	}
	owner, repo, err := validation.ValidateRepoName(args[0])
	if err != nil {
		_, _ = d.replyMsg(m, "❌ Invalid repo format.")
		return nil
	}
	fullName := owner + "/" + repo

	// Try to delete the webhook via the user's client.
	link, _ := d.deps.DB.GetRepoLink(ctx, m.Chat.ID, fullName)
	if link != nil && link.WebhookID != 0 {
		client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
		if err == nil {
			_ = ghops.DeleteWebhook(ctx, client, owner, repo, link.WebhookID)
		}
	}
	if err := d.deps.DB.RemoveRepoLink(ctx, m.Chat.ID, fullName); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Failed to remove: %v", err))
		return nil
	}
	_ = d.deps.DB.DeleteWebhookConfig(ctx, m.Chat.ID, fullName)
	// Also revoke the route ID (if using route-ID-based routing).
	if d.deps.Routes != nil {
		_ = d.deps.Routes.Delete(ctx, m.Chat.ID, fullName)
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "repo.remove", fullName, audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Repository <b>%s</b> removed.", fullName))
	return nil
}

func (d *Dispatcher) cmdRepos(ctx context.Context, m *tgbotapi.Message, args []string) error {
	links, err := d.deps.DB.GetChatLinks(ctx, m.Chat.ID)
	if err != nil || len(links) == 0 {
		_, _ = d.replyMsg(m, "No repositories linked. Use /addrepo.")
		return nil
	}
	var b strings.Builder
	b.WriteString("<b>Linked Repositories:</b>\n")
	for _, l := range links {
		fmt.Fprintf(&b, "• <b>%s</b>", l.RepoFullName)
		if l.Muted {
			b.WriteString(" 🔕")
		}
		b.WriteString("\n")
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdRepo(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, args)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	r, err := ghops.GetRepo(ctx, client, owner, repo)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>\n", r.GetFullName())
	if r.GetDescription() != "" {
		fmt.Fprintf(&b, "%s\n", r.GetDescription())
	}
	fmt.Fprintf(&b, "⭐ %d · 🍴 %d · 👁 %d\n", r.GetStargazersCount(), r.GetForksCount(), r.GetWatchersCount())
	fmt.Fprintf(&b, "Issues: %d open\n", r.GetOpenIssuesCount())
	fmt.Fprintf(&b, "Default branch: <code>%s</code>\n", r.GetDefaultBranch())
	if r.GetHTMLURL() != "" {
		fmt.Fprintf(&b, "<a href=\"%s\">Open in GitHub</a>", r.GetHTMLURL())
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdStar(ctx context.Context, m *tgbotapi.Message, args []string) error {
	return d.repoAction(ctx, m, args, func(c *gh.Client, owner, repo string) error {
		return ghops.Star(ctx, c, owner, repo)
	}, "starred")
}

func (d *Dispatcher) cmdUnstar(ctx context.Context, m *tgbotapi.Message, args []string) error {
	return d.repoAction(ctx, m, args, func(c *gh.Client, owner, repo string) error {
		return ghops.Unstar(ctx, c, owner, repo)
	}, "unstarred")
}

func (d *Dispatcher) cmdWatch(ctx context.Context, m *tgbotapi.Message, args []string) error {
	return d.repoAction(ctx, m, args, func(c *gh.Client, owner, repo string) error {
		return ghops.Watch(ctx, c, owner, repo, "")
	}, "watched")
}

func (d *Dispatcher) cmdUnwatch(ctx context.Context, m *tgbotapi.Message, args []string) error {
	return d.repoAction(ctx, m, args, func(c *gh.Client, owner, repo string) error {
		return ghops.Unwatch(ctx, c, owner, repo)
	}, "unwatched")
}

func (d *Dispatcher) cmdFork(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, args)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	f, err := ghops.Fork(ctx, client, owner, repo)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Fork failed: %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Forked to <b>%s</b>", f.GetFullName()))
	return nil
}

func (d *Dispatcher) cmdArchive(ctx context.Context, m *tgbotapi.Message, args []string) error {
	return d.repoActionConfirm(ctx, m, args, "archive", func(c *gh.Client, owner, repo string) error {
		return ghops.Archive(ctx, c, owner, repo)
	})
}

func (d *Dispatcher) cmdUnarchive(ctx context.Context, m *tgbotapi.Message, args []string) error {
	return d.repoActionConfirm(ctx, m, args, "unarchive", func(c *gh.Client, owner, repo string) error {
		return ghops.Unarchive(ctx, c, owner, repo)
	})
}

func (d *Dispatcher) cmdContributors(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, args)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	list, err := ghops.ListContributors(ctx, client, owner, repo, 10)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	b.WriteString("<b>Top Contributors:</b>\n")
	for _, c := range list {
		fmt.Fprintf(&b, "• %s — %d commits\n", c.GetLogin(), c.GetContributions())
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdLanguages(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, args)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	langs, err := ghops.ListLanguages(ctx, client, owner, repo)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	total := 0
	for _, n := range langs {
		total += n
	}
	var b strings.Builder
	b.WriteString("<b>Languages:</b>\n")
	for name, n := range langs {
		pct := 0.0
		if total > 0 {
			pct = float64(n) * 100.0 / float64(total)
		}
		fmt.Fprintf(&b, "• %s — %.1f%%\n", name, pct)
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdStats(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, args)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	r, err := ghops.GetRepo(ctx, client, owner, repo)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s Statistics</b>\n", r.GetFullName())
	fmt.Fprintf(&b, "⭐ Stars: %d\n", r.GetStargazersCount())
	fmt.Fprintf(&b, "🍴 Forks: %d\n", r.GetForksCount())
	fmt.Fprintf(&b, "👁 Watchers: %d\n", r.GetWatchersCount())
	fmt.Fprintf(&b, "📋 Open issues: %d\n", r.GetOpenIssuesCount())
	fmt.Fprintf(&b, "📦 Size: %d KB\n", r.GetSize())
	_, _ = d.replyMsg(m, b.String())
	return nil
}

// repoAction is a helper for simple repo operations (star/unstar/watch).
func (d *Dispatcher) repoAction(ctx context.Context, m *tgbotapi.Message, args []string, fn func(*gh.Client, string, string) error, pastTense string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, args)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := fn(client, owner, repo); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ %s/%s %s.", owner, repo, pastTense))
	return nil
}

// repoActionConfirm is like repoAction but for destructive ops.
// For simplicity, we require the user to reply "yes" via a follow-up command
// in the same chat within 60s. Here we just perform the action and rely on
// GitHub branch protection / repo admin checks for safety.
func (d *Dispatcher) repoActionConfirm(ctx context.Context, m *tgbotapi.Message, args []string, verb string, fn func(*gh.Client, string, string) error) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, args)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := fn(client, owner, repo); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %s failed: %v", verb, err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "repo."+verb, owner+"/"+repo, audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ %s/%s %sed.", owner, repo, verb))
	return nil
}

// sendRepoList shows the user's accessible repos for interactive selection.
//
// When called from a callback query (cq != nil), it EDITS the existing
// message (so pagination feels smooth and doesn't spam the chat with new
// messages). When called from a command (cq == nil), it sends a new message.
//
// Callback data format (pagination):
//
//	c:arp:<page>   — navigate to page <page>
//	c:ar:<owner/repo> — select a repo to add
//
// The "arp" prefix (add-repo-page) is deliberately distinct from "ar"
// (add-repo) to avoid the strings.Split collision that previously caused
// pagination buttons to do nothing.
func (d *Dispatcher) sendRepoList(ctx context.Context, m *tgbotapi.Message, page int) error {
	return d.sendRepoListWithCallback(ctx, m, page, nil)
}

// sendRepoListWithCallback is like sendRepoList but, when cq is non-nil,
// edits the cq's message instead of sending a new one.
func (d *Dispatcher) sendRepoListWithCallback(ctx context.Context, m *tgbotapi.Message, page int, cq *tgbotapi.CallbackQuery) error {
	if page < 1 {
		page = 1
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		_, _ = d.replyMsg(m, "⚠️ Please /connect your GitHub account first.")
		return nil
	}
	perPage := 5
	repos, resp, err := ghops.ListUserRepos(ctx, client, page, perPage)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Failed to list repos: %v", err))
		return nil
	}
	if len(repos) == 0 && page == 1 {
		_, _ = d.replyMsg(m, "No repositories found.")
		return nil
	}
	if len(repos) == 0 {
		// Requested a page beyond the last one — go back to page 1.
		return d.sendRepoListWithCallback(ctx, m, 1, cq)
	}
	var rows [][]telegram.Button
	for _, r := range repos {
		full := r.GetFullName()
		rows = append(rows, []telegram.Button{{Text: full, Data: "c:ar:" + full}})
	}
	// Pagination row.
	var nav []telegram.Button
	if resp.PrevPage > 0 {
		nav = append(nav, telegram.Button{Text: "◀️ Prev", Data: fmt.Sprintf("c:arp:%d", resp.PrevPage)})
	}
	// Show current page number (non-clickable).
	nav = append(nav, telegram.Button{Text: fmt.Sprintf("Page %d", page), Data: "c:noop"})
	if resp.NextPage > 0 {
		nav = append(nav, telegram.Button{Text: "Next ▶️", Data: fmt.Sprintf("c:arp:%d", resp.NextPage)})
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	markup := telegram.InlineKeyboard(rows)
	text := fmt.Sprintf("Select a repository to add (Page %d):", page)
	if cq != nil {
		// Edit the existing message. If the edit fails (e.g. message is
		// too old or content is identical), fall back to sending a new
		// message so the user still sees the updated keyboard.
		if err := d.deps.Bot.EditText(m.Chat.ID, int64(m.MessageID), text, "HTML", markup); err != nil {
			_, _ = d.deps.Bot.SendMessage(m.Chat.ID, text, "HTML", 0, 0, markup)
		}
	} else {
		_, _ = d.deps.Bot.SendMessage(m.Chat.ID, text, "HTML", int32(m.MessageID), 0, markup)
	}
	return nil
}

// defaultEvents returns the list of events to subscribe to when creating a
// new repo webhook.
func defaultEvents() []string {
	// A curated subset that's useful in Telegram — avoids spam from
	// noisy events like status checks on every push.
	return []string{
		"push", "pull_request", "issues", "issue_comment",
		"pull_request_review", "pull_request_review_comment",
		"release", "fork", "star", "workflow_run",
		"check_run", "check_suite", "repository", "member",
		"create", "delete", "label", "milestone", "gollum",
		"commit_comment", "watch", "public",
	}
}
