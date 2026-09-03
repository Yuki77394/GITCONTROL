package commands

import (
	"context"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"strings"

	"github.com/swaggymusic/github-bot/internal/ghops"
)

// registerSearch registers search commands.
func (d *Dispatcher) registerSearch() {
	d.Register("find", Handler{Run: d.cmdFind, HelpText: "Search issues."})
	d.Register("pr", Handler{Run: d.cmdPRSearch, HelpText: "Search pull requests."})
	d.Register("search", Handler{Run: d.cmdSearchCode, HelpText: "Search code."})
}

func (d *Dispatcher) cmdFind(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/find &lt;query&gt;</code>")
		return nil
	}
	q := strings.Join(args, " ")
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	res, err := ghops.SearchIssues(ctx, client, owner, repo, q, 1, 10)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	if res.GetTotal() == 0 {
		_, _ = d.replyMsg(m, "No issues found.")
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%d issues found:</b>\n", res.GetTotal())
	for _, i := range res.Issues {
		fmt.Fprintf(&b, "• #%d %s\n", i.GetNumber(), i.GetTitle())
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdPRSearch(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/pr &lt;query&gt;</code>")
		return nil
	}
	q := strings.Join(args, " ")
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	res, err := ghops.SearchPRs(ctx, client, owner, repo, q, 1, 10)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	if res.GetTotal() == 0 {
		_, _ = d.replyMsg(m, "No PRs found.")
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%d PRs found:</b>\n", res.GetTotal())
	for _, i := range res.Issues {
		fmt.Fprintf(&b, "• #%d %s\n", i.GetNumber(), i.GetTitle())
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdSearchCode(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/search &lt;query&gt;</code>")
		return nil
	}
	q := strings.Join(args, " ")
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	res, err := ghops.SearchCode(ctx, client, owner, repo, q, 1, 10)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v\n\nNote: code search requires the user's token to have search scope.", err))
		return nil
	}
	if res.GetTotal() == 0 {
		_, _ = d.replyMsg(m, "No code results.")
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%d code results:</b>\n", res.GetTotal())
	for _, c := range res.CodeResults {
		fmt.Fprintf(&b, "• <a href=\"%s\">%s</a>\n", c.GetHTMLURL(), c.GetPath())
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}
