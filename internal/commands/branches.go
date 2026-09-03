package commands

import (
	"context"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"strings"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/ghops"
	"github.com/swaggymusic/github-bot/internal/validation"
)

// registerBranches registers branch management commands.
func (d *Dispatcher) registerBranches() {
	d.Register("branches", Handler{Run: d.cmdBranches, HelpText: "List branches."})
	d.Register("branch", Handler{Run: d.cmdBranch, HelpText: "Show branch info."})
	d.Register("createbranch", Handler{Run: d.cmdCreateBranch, HelpText: "Create a branch."})
	d.Register("deletebranch", Handler{Run: d.cmdDeleteBranch, HelpText: "Delete a branch (admin).", AdminOnly: true})
	d.Register("default", Handler{Run: d.cmdDefaultBranch, HelpText: "Change default branch (admin).", AdminOnly: true})
}

func (d *Dispatcher) cmdBranches(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	list, _, err := ghops.ListBranches(ctx, client, owner, repo, 1, 20)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	b.WriteString("<b>Branches:</b>\n")
	for _, br := range list {
		fmt.Fprintf(&b, "• <code>%s</code>\n", br.GetName())
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdBranch(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/branch &lt;name&gt;</code>")
		return nil
	}
	branch := args[0]
	if err := validation.ValidateBranchName(branch); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	br, err := ghops.GetBranch(ctx, client, owner, repo, branch)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Branch: %s</b>\n", br.GetName())
	fmt.Fprintf(&b, "Protected: %v\n", br.GetProtected())
	if br.GetCommit() != nil {
		fmt.Fprintf(&b, "Latest commit: <code>%s</code>\n", br.GetCommit().GetSHA()[:7])
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdCreateBranch(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 2 {
		_, _ = d.replyMsg(m, "Usage: <code>/createbranch &lt;new&gt; &lt;from&gt;</code>")
		return nil
	}
	newName := args[0]
	fromName := args[1]
	if err := validation.ValidateBranchName(newName); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	if err := validation.ValidateBranchName(fromName); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.CreateBranch(ctx, client, owner, repo, newName, fromName); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Branch <code>%s</code> created from <code>%s</code>.", newName, fromName))
	return nil
}

func (d *Dispatcher) cmdDeleteBranch(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/deletebranch &lt;name&gt;</code>")
		return nil
	}
	branch := args[0]
	if err := validation.ValidateBranchName(branch); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.DeleteBranch(ctx, client, owner, repo, branch); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "branch.delete", fmt.Sprintf("%s/%s:%s", owner, repo, branch), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Branch <code>%s</code> deleted.", branch))
	return nil
}

func (d *Dispatcher) cmdDefaultBranch(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/default &lt;branch&gt;</code>")
		return nil
	}
	branch := args[0]
	if err := validation.ValidateBranchName(branch); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.SetDefaultBranch(ctx, client, owner, repo, branch); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "branch.set_default", fmt.Sprintf("%s/%s:%s", owner, repo, branch), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ Default branch set to <code>%s</code>.", branch))
	return nil
}
