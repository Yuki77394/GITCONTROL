package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/swaggymusic/github-bot/internal/audit"
	"github.com/swaggymusic/github-bot/internal/ghops"
	"github.com/swaggymusic/github-bot/internal/validation"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	gh "github.com/google/go-github/v66/github"
)

// registerFiles registers file management commands.
func (d *Dispatcher) registerFiles() {
	d.Register("ls", Handler{Run: d.cmdLs, HelpText: "List directory contents."})
	d.Register("cat", Handler{Run: d.cmdCat, HelpText: "View a text file."})
	d.Register("createfile", Handler{Run: d.cmdCreateFile, HelpText: "Create a file."})
	d.Register("updatefile", Handler{Run: d.cmdUpdateFile, HelpText: "Update an existing file."})
	d.Register("deletefile", Handler{Run: d.cmdDeleteFile, HelpText: "Delete a file (admin).", AdminOnly: true})
}

// resolveDefaultBranch fetches the repository's default branch (e.g. "main",
// "master", "develop") instead of hard-coding "main". Falls back to "main"
// if the API call fails.
func (d *Dispatcher) resolveDefaultBranch(ctx context.Context, client *gh.Client, owner, repo string) string {
	r, err := ghops.GetRepo(ctx, client, owner, repo)
	if err != nil {
		return "main"
	}
	if db := r.GetDefaultBranch(); db != "" {
		return db
	}
	return "main"
}

func (d *Dispatcher) cmdLs(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	if err := validation.ValidateFilePath(path); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	entries, err := ghops.ListDir(ctx, client, owner, repo, d.resolveDefaultBranch(ctx, client, owner, repo), path)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Contents of %s:</b>\n", path)
	for _, e := range entries {
		typ := "📄"
		if e.GetType() == "dir" {
			typ = "📁"
		}
		fmt.Fprintf(&b, "%s <code>%s</code>\n", typ, e.GetName())
	}
	_, _ = d.replyMsg(m, b.String())
	return nil
}

func (d *Dispatcher) cmdCat(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/cat &lt;path&gt;</code>")
		return nil
	}
	path := args[0]
	if err := validation.ValidateFilePath(path); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	_, content, err := ghops.GetFileContent(ctx, client, owner, repo, d.resolveDefaultBranch(ctx, client, owner, repo), path)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	// Limit display size.
	if len(content) > 3500 {
		content = append(content[:3500], []byte("\n… (truncated)")...)
	}
	// Wrap in <pre> for monospace rendering.
	_, _ = d.replyMsg(m, fmt.Sprintf("<b>%s</b>:\n<pre>%s</pre>", path, escapeHTML(string(content))))
	return nil
}

func (d *Dispatcher) cmdCreateFile(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 3 {
		_, _ = d.replyMsg(m, "Usage: <code>/createfile &lt;path&gt; &lt;commit_message&gt; &lt;content&gt;</code>")
		return nil
	}
	path := args[0]
	msg := args[1]
	content := strings.Join(args[2:], " ")
	if err := validation.ValidateFilePath(path); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	if err := ghops.CreateFile(ctx, client, owner, repo, d.resolveDefaultBranch(ctx, client, owner, repo), path, msg, content); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "file.create", fmt.Sprintf("%s/%s:%s", owner, repo, path), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ File <code>%s</code> created.", path))
	return nil
}

// cmdUpdateFile updates an existing repository file.
//
// Usage:
//
//	/updatefile <path> <content>                — uses default commit message
//	/updatefile <path> <commit_message> <content> — explicit commit message
//
// The command:
//  1. Validates repo selection and GitHub auth (resolveRepoFromContext +
//     GetDecryptedClient).
//  2. Validates the file path (no .., no absolute paths, no NUL bytes).
//  3. Fetches the existing file (including its current blob SHA, which the
//     GitHub Contents API requires for updates).
//  4. Calls ghops.UpdateFile with the new content.
//  5. Returns a clear success/failure message.
//
// For very large content, the command suggests using the GitHub web UI URL
// instead, to avoid Telegram's 4096-character message limit.
//
// Security: the bot never executes repository files locally and never
// accesses the local server filesystem. All operations go through the
// GitHub Contents API.
func (d *Dispatcher) cmdUpdateFile(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 2 {
		_, _ = d.replyMsg(m, "Usage: <code>/updatefile &lt;path&gt; [commit_message] &lt;content&gt;</code>\n\nExample:\n<code>/updatefile README.md \"Update readme\" New content here</code>\n<code>/updatefile README.md New content here</code>")
		return nil
	}
	path := args[0]
	if err := validation.ValidateFilePath(path); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	// Parse commit message vs. content.
	// If the user passed 3+ args, treat args[1] as the commit message.
	// Otherwise (2 args), use a default commit message.
	commitMsg := "Update " + path
	content := strings.Join(args[1:], " ")
	if len(args) >= 3 {
		commitMsg = args[1]
		content = strings.Join(args[2:], " ")
	}
	// Defensive: cap content size to keep Telegram and GitHub happy.
	const maxContentBytes = 95000 // GitHub limit is 100 MB but Telegram-bound
	if len(content) > maxContentBytes {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Content too large (%d bytes, max %d). For large files, edit via GitHub web UI.", len(content), maxContentBytes))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	branch := d.resolveDefaultBranch(ctx, client, owner, repo)
	// Fetch the existing file to get its current SHA (required by the
	// GitHub Contents API for updates).
	file, _, err := ghops.GetFileContent(ctx, client, owner, repo, branch, path)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ Could not fetch existing file: %v", err))
		return nil
	}
	if file == nil {
		_, _ = d.replyMsg(m, "❌ File not found. Use /createfile to create it.")
		return nil
	}
	sha := file.GetSHA()
	if sha == "" {
		_, _ = d.replyMsg(m, "❌ Existing file has no SHA (cannot update).")
		return nil
	}
	if err := ghops.UpdateFile(ctx, client, owner, repo, branch, path, commitMsg, content, sha); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "file.update", fmt.Sprintf("%s/%s:%s", owner, repo, path), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ File <code>%s</code> updated (commit: <code>%s</code>).", path, commitMsg))
	return nil
}

func (d *Dispatcher) cmdDeleteFile(ctx context.Context, m *tgbotapi.Message, args []string) error {
	owner, repo, err := d.resolveRepoFromContext(ctx, m, nil)
	if err != nil {
		_, _ = d.replyMsg(m, err.Error())
		return nil
	}
	if len(args) < 1 {
		_, _ = d.replyMsg(m, "Usage: <code>/deletefile &lt;path&gt;</code>")
		return nil
	}
	path := args[0]
	if err := validation.ValidateFilePath(path); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	client, _, err := d.deps.Access.GetDecryptedClient(ctx, m.From.ID)
	if err != nil {
		return err
	}
	// Fetch the current SHA.
	file, _, err := ghops.GetFileContent(ctx, client, owner, repo, d.resolveDefaultBranch(ctx, client, owner, repo), path)
	if err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	if err := ghops.DeleteFile(ctx, client, owner, repo, d.resolveDefaultBranch(ctx, client, owner, repo), path, "Delete "+path, file.GetSHA()); err != nil {
		_, _ = d.replyMsg(m, fmt.Sprintf("❌ %v", err))
		return nil
	}
	d.deps.Audit.Log(ctx, m.From.ID, m.From.UserName, "file.delete", fmt.Sprintf("%s/%s:%s", owner, repo, path), audit.ResultSuccess, "", m.Chat.ID)
	_, _ = d.replyMsg(m, fmt.Sprintf("✅ File <code>%s</code> deleted.", path))
	return nil
}

// escapeHTML replaces &, <, > with HTML entities for safe inclusion in
// <pre> blocks (Telegram HTML parse mode).
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
