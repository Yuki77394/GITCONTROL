package commands

import (
	"context"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"strings"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/ghops"
)

// registerReleases registers release management commands.
func (d *Dispatcher) registerReleases() {
	d.Register("release", Handler{Run: d.cmdRelease, HelpText: "Show latest release or create one."})
	d.Register("changelog", Handler{Run: d.cmdChangelog, HelpText: "Generate release notes."})
}

func (d *Dispatcher) cmdRelease(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if len(args) >= 2 && args[0] == "create" {
		tag := args[1]
		if tag == "" {
			_, _ = d.replyMsg(m, "Usage: <code>/release create &lt;tag&gt;</code>")
			return nil
		}
		r, err := ghops.CreateRelease(ctx, client, owner, repo, tag, tag, "", "main", false, false, true)
		if err != nil {
			_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
			return nil
		}
		d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "release.create", fmt.Sprintf("%s/%s:%s", owner, repo, tag), audit.ResultSuccess, "", m.Chat.ID)
		_, _ = d.replyMsg(m, fmt.Sprintf("✅ Release <b>%s</b> created: %s", tag, r.GetHTMLURL()))
		return nil
	}
	// Default: show latest.
	r, err := ghops.GetLatestRelease(ctx, client, owner, repo)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Latest Release: %s</b>\n", r.GetTagName())
	if r.GetName() != "" {
		fmt.Fprintf(&b, "%s\n", r.GetName())
	}
	if r.GetBody() != "" {
		body := r.GetBody()
		if len(body) > 1000 {
			body = body[:1000] + "…"
		}
		fmt.Fprintf(&b, "\n%s\n", body)
	}
	if r.GetHTMLURL() != "" {
		fmt.Fprintf(&b, "<a href=\"%s\">View release</a>", r.GetHTMLURL())
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdChangelog(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/changelog &lt;tag&gt; [previous_tag]</code>")
		return nil
	}
	tag := args[0]
	previous := ""
	if len(args) > 1 {
		previous = args[1]
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	body, err := ghops.GenerateReleaseNotes(ctx, client, owner, repo, tag, previous)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	if len(body) > 3500 {
		body = body[:3500] + "…"
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("<b>Changelog for %s:</b>\n\n%s", tag, body))
	return nil
}
