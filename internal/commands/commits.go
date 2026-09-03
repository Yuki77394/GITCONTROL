package commands

import (
	"context"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"strings"

	"github.com/swaggymusic/github-bot/internal/ghops"
	"github.com/swaggymusic/github-bot/internal/validation"
)

// registerCommits registers commit-related commands.
func (d *Dispatcher) registerCommits() {
	d.Register("commit", Handler{Run: d.cmdCommit, HelpText: "Show commit details."})
	d.Register("commits", Handler{Run: d.cmdCommits, HelpText: "Recent commits."})
	d.Register("compare", Handler{Run: d.cmdCompare, HelpText: "Compare two refs."})
}

func (d *Dispatcher) cmdCommit(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/commit &lt;SHA&gt;</code>")
		return nil
	}
	sha := args[0]
	if err := validation.ValidateSHA(sha); err != nil {
		_, _ = d.replyMsg(m, "❌ Invalid SHA.")
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	c, err := ghops.GetCommit(ctx, client, owner, repo, sha)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Commit %s</b>\n", c.GetSHA()[:7])
	fmt.Fprintf(&b, "Author: %s\n", c.GetCommit().GetAuthor().GetName())
	if c.GetCommit().GetMessage() != "" {
		msg := strings.SplitN(c.GetCommit().GetMessage(), "\n", 2)[0]
		fmt.Fprintf(&b, "Message: %s\n", msg)
	}
	fmt.Fprintf(&b, "Files: %d  +%d/-%d\n", c.GetStats().GetTotal(), c.GetStats().GetAdditions(), c.GetStats().GetDeletions())
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdCommits(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	branch := "main"
	if len(args) > 0 {
		branch = args[0]
	}
	if err := validation.ValidateBranchName(branch); err != nil {
		_, _ = d.replyMsg(m, "❌ Invalid branch name.")
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	list, err := ghops.ListCommits(ctx, client, owner, repo, branch, 1, 10)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Recent commits on %s:</b>\n", branch)
	for _, c := range list {
		sha := c.GetSHA()
		if len(sha) > 7 {
			sha = sha[:7]
		}
		msg := strings.SplitN(c.GetCommit().GetMessage(), "\n", 2)[0]
		fmt.Fprintf(&b, "• <code>%s</code> %s\n", sha, msg)
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdCompare(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 2 {
		_, _ = d.replyMsg(m, "Usage: <code>/compare &lt;base&gt; &lt;head&gt;</code>")
		return nil
	}
	base := args[0]
	head := args[1]
	if err := validation.ValidateBranchName(base); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Invalid base: %v", err))
		return nil
	}
	if err := validation.ValidateBranchName(head); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Invalid head: %v", err))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	cmp, err := ghops.Compare(ctx, client, owner, repo, base, head)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Comparison %s...%s</b>\n", base, head)
	fmt.Fprintf(&b, "Commits: %d  Files: %d\n", cmp.GetTotalCommits(), len(cmp.Files))
	_, _ = d.replyMsg(m, b.String())
	return nil
}
