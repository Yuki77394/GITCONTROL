package webhooks

import (
	"fmt"
	"strings"

	gh "github.com/google/go-github/v66/github"
)

func formatPush(e *gh.PushEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	pusher := e.GetPusher().GetName()
	branch := strings.TrimPrefix(e.GetRef(), "refs/heads/")
	commits := e.GetCommits()
	compare := e.GetCompare()

	var b strings.Builder
	fmt.Fprintf(&b, `<b>Push</b> · %s → <code>%s</code>`+"\n", repoLink(owner, name), esc(branch))
	if pusher != "" {
		fmt.Fprintf(&b, "by %s\n", esc(pusher))
	}
	fmt.Fprintf(&b, "%d commit(s)\n", len(commits))
	for i, c := range commits {
		if i >= 5 {
			fmt.Fprintf(&b, "… and %d more\n", len(commits)-5)
			break
		}
		sha := c.GetID()
		if len(sha) > 7 {
			sha = sha[:7]
		}
		msg := strings.SplitN(c.GetMessage(), "\n", 2)[0]
		fmt.Fprintf(&b, "• <code>%s</code> %s\n", esc(sha), esc(truncate(msg, 80)))
	}
	if compare != "" {
		fmt.Fprintf(&b, `<a href="%s">View comparison</a>`, esc(compare))
	}
	return b.String()
}

func formatPullRequest(e *gh.PullRequestEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	pr := e.GetPullRequest()
	action := e.GetAction()
	num := pr.GetNumber()
	title := pr.GetTitle()
	user := pr.GetUser().GetLogin()
	htmlURL := pr.GetHTMLURL()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Pull Request #%d %s</b>\n", num, esc(action))
	fmt.Fprintf(&b, "%s · %s\n", repoLink(owner, name), esc(title))
	fmt.Fprintf(&b, "by %s\n", userLink(user))
	if htmlURL != "" {
		fmt.Fprintf(&b, `<a href="%s">Open PR</a>`, esc(htmlURL))
	}
	return b.String()
}

func formatIssues(e *gh.IssuesEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	issue := e.GetIssue()
	action := e.GetAction()
	num := issue.GetNumber()
	title := issue.GetTitle()
	user := issue.GetUser().GetLogin()
	htmlURL := issue.GetHTMLURL()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Issue #%d %s</b>\n", num, esc(action))
	fmt.Fprintf(&b, "%s · %s\n", repoLink(owner, name), esc(title))
	fmt.Fprintf(&b, "by %s\n", userLink(user))
	if htmlURL != "" {
		fmt.Fprintf(&b, `<a href="%s">Open issue</a>`, esc(htmlURL))
	}
	return b.String()
}

func formatIssueComment(e *gh.IssueCommentEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	issue := e.GetIssue()
	comment := e.GetComment()
	user := comment.GetUser().GetLogin()
	body := truncate(comment.GetBody(), 500)

	var b strings.Builder
	fmt.Fprintf(&b, "<b>New comment</b> on #%d\n", issue.GetNumber())
	fmt.Fprintf(&b, "%s · %s\n", repoLink(owner, name), esc(issue.GetTitle()))
	fmt.Fprintf(&b, "by %s\n", userLink(user))
	if body != "" {
		fmt.Fprintf(&b, "\n%s", esc(body))
	}
	return b.String()
}

func formatPRReview(e *gh.PullRequestReviewEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	pr := e.GetPullRequest()
	review := e.GetReview()
	user := review.GetUser().GetLogin()
	state := review.GetState()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>PR Review</b> · %s\n", esc(state))
	fmt.Fprintf(&b, "%s · PR #%d %s\n", repoLink(owner, name), pr.GetNumber(), esc(pr.GetTitle()))
	fmt.Fprintf(&b, "by %s\n", userLink(user))
	if review.GetBody() != "" {
		fmt.Fprintf(&b, "\n%s", esc(truncate(review.GetBody(), 500)))
	}
	return b.String()
}

func formatPRReviewComment(e *gh.PullRequestReviewCommentEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	pr := e.GetPullRequest()
	comment := e.GetComment()
	user := comment.GetUser().GetLogin()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Review comment</b> on PR #%d\n", pr.GetNumber())
	fmt.Fprintf(&b, "%s · %s\n", repoLink(owner, name), esc(pr.GetTitle()))
	fmt.Fprintf(&b, "by %s\n", userLink(user))
	if comment.GetBody() != "" {
		fmt.Fprintf(&b, "\n%s", esc(truncate(comment.GetBody(), 500)))
	}
	return b.String()
}

func formatRelease(e *gh.ReleaseEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	rel := e.GetRelease()
	action := e.GetAction()
	tag := rel.GetTagName()
	user := rel.GetAuthor().GetLogin()
	htmlURL := rel.GetHTMLURL()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Release %s</b> · <code>%s</code>\n", esc(action), esc(tag))
	fmt.Fprintf(&b, "%s\n", repoLink(owner, name))
	fmt.Fprintf(&b, "by %s\n", userLink(user))
	if htmlURL != "" {
		fmt.Fprintf(&b, `<a href="%s">View release</a>`, esc(htmlURL))
	}
	return b.String()
}

func formatFork(e *gh.ForkEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	forkee := e.GetForkee()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Fork created</b>\n")
	fmt.Fprintf(&b, "%s forked by %s\n", repoLink(owner, name), userLink(forkee.GetOwner().GetLogin()))
	if forkee.GetHTMLURL() != "" {
		fmt.Fprintf(&b, `<a href="%s">View fork</a>`, esc(forkee.GetHTMLURL()))
	}
	return b.String()
}

func formatStar(e *gh.StarEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	sender := e.GetSender().GetLogin()

	var b strings.Builder
	fmt.Fprintf(&b, "⭐ <b>Starred</b>\n")
	fmt.Fprintf(&b, "%s by %s\n", repoLink(owner, name), userLink(sender))
	return b.String()
}

func formatWatch(e *gh.WatchEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	sender := e.GetSender().GetLogin()

	var b strings.Builder
	fmt.Fprintf(&b, "👁 <b>Watched</b>\n")
	fmt.Fprintf(&b, "%s by %s\n", repoLink(owner, name), userLink(sender))
	return b.String()
}

func formatWorkflowRun(e *gh.WorkflowRunEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	run := e.GetWorkflowRun()
	wf := e.GetWorkflow()
	status := run.GetStatus()
	conclusion := run.GetConclusion()
	branch := run.GetHeadBranch()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Workflow Run</b> · %s\n", esc(status))
	if conclusion != "" {
		fmt.Fprintf(&b, "Conclusion: <code>%s</code>\n", esc(conclusion))
	}
	fmt.Fprintf(&b, "%s\n", repoLink(owner, name))
	if wf != nil {
		fmt.Fprintf(&b, "Workflow: %s\n", esc(wf.GetName()))
	}
	fmt.Fprintf(&b, "Branch: <code>%s</code>\n", esc(branch))
	if run.GetHTMLURL() != "" {
		fmt.Fprintf(&b, `<a href="%s">View run</a>`, esc(run.GetHTMLURL()))
	}
	return b.String()
}

func formatCheckRun(e *gh.CheckRunEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	cr := e.GetCheckRun()
	status := cr.GetStatus()
	conclusion := cr.GetConclusion()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Check Run</b> · %s\n", esc(cr.GetName()))
	fmt.Fprintf(&b, "%s\n", repoLink(owner, name))
	fmt.Fprintf(&b, "Status: <code>%s</code>", esc(status))
	if conclusion != "" {
		fmt.Fprintf(&b, " · Conclusion: <code>%s</code>", esc(conclusion))
	}
	return b.String()
}

func formatCheckSuite(e *gh.CheckSuiteEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	cs := e.GetCheckSuite()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Check Suite</b>\n")
	fmt.Fprintf(&b, "%s\n", repoLink(owner, name))
	fmt.Fprintf(&b, "Status: <code>%s</code>", esc(cs.GetStatus()))
	if cs.GetConclusion() != "" {
		fmt.Fprintf(&b, " · Conclusion: <code>%s</code>", esc(cs.GetConclusion()))
	}
	return b.String()
}

func formatRepository(e *gh.RepositoryEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	action := e.GetAction()
	sender := e.GetSender().GetLogin()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Repository %s</b>\n", esc(action))
	fmt.Fprintf(&b, "%s\n", repoLink(owner, name))
	fmt.Fprintf(&b, "by %s", userLink(sender))
	return b.String()
}

func formatCreate(e *gh.CreateEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	refType := e.GetRefType()
	ref := e.GetRef()
	sender := e.GetSender().GetLogin()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s created</b> · <code>%s</code>\n", esc(refType), esc(ref))
	fmt.Fprintf(&b, "%s by %s", repoLink(owner, name), userLink(sender))
	return b.String()
}

func formatDelete(e *gh.DeleteEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	refType := e.GetRefType()
	ref := e.GetRef()
	sender := e.GetSender().GetLogin()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s deleted</b> · <code>%s</code>\n", esc(refType), esc(ref))
	fmt.Fprintf(&b, "%s by %s", repoLink(owner, name), userLink(sender))
	return b.String()
}

func formatMember(e *gh.MemberEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	action := e.GetAction()
	member := e.GetMember().GetLogin()
	sender := e.GetSender().GetLogin()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Collaborator %s</b>\n", esc(action))
	fmt.Fprintf(&b, "%s\n", repoLink(owner, name))
	fmt.Fprintf(&b, "%s by %s", userLink(member), userLink(sender))
	return b.String()
}

func formatLabel(e *gh.LabelEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	action := e.GetAction()
	label := e.GetLabel().GetName()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Label %s</b> · <code>%s</code>\n", esc(action), esc(label))
	fmt.Fprintf(&b, "%s", repoLink(owner, name))
	return b.String()
}

func formatMilestone(e *gh.MilestoneEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	action := e.GetAction()
	m := e.GetMilestone()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Milestone %s</b> · %s\n", esc(action), esc(m.GetTitle()))
	fmt.Fprintf(&b, "%s", repoLink(owner, name))
	return b.String()
}

func formatGollum(e *gh.GollumEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	sender := e.GetSender().GetLogin()
	pages := e.Pages

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Wiki updated</b>\n")
	fmt.Fprintf(&b, "%s by %s\n", repoLink(owner, name), userLink(sender))
	fmt.Fprintf(&b, "%d page(s) changed\n", len(pages))
	for i, p := range pages {
		if i >= 5 {
			break
		}
		fmt.Fprintf(&b, "• %s\n", esc(p.GetPageName()))
	}
	return b.String()
}

func formatCommitComment(e *gh.CommitCommentEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	comment := e.GetComment()
	user := comment.GetUser().GetLogin()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Commit comment</b>\n")
	fmt.Fprintf(&b, "%s\n", repoLink(owner, name))
	fmt.Fprintf(&b, "by %s\n", userLink(user))
	if comment.GetBody() != "" {
		fmt.Fprintf(&b, "\n%s", esc(truncate(comment.GetBody(), 500)))
	}
	return b.String()
}

func formatStatus(e *gh.StatusEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	state := e.GetState()
	sha := e.GetCommit().GetSHA()
	if len(sha) > 7 {
		sha = sha[:7]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Commit status</b> · <code>%s</code>\n", esc(state))
	fmt.Fprintf(&b, "%s · <code>%s</code>", repoLink(owner, name), esc(sha))
	return b.String()
}

func formatPublic(e *gh.PublicEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	sender := e.GetSender().GetLogin()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Repository made public</b>\n")
	fmt.Fprintf(&b, "%s by %s", repoLink(owner, name), userLink(sender))
	return b.String()
}

func formatMembership(payload interface{}) string {
	switch e := payload.(type) {
	case *gh.TeamEvent:
		return fmt.Sprintf("<b>Team event</b> · %s", esc(e.GetAction()))
	case *gh.TeamAddEvent:
		return fmt.Sprintf("<b>Team added</b> to repository")
	case *gh.MembershipEvent:
		return fmt.Sprintf("<b>Membership</b> · %s", esc(e.GetAction()))
	case *gh.OrganizationEvent:
		return fmt.Sprintf("<b>Organization event</b> · %s", esc(e.GetAction()))
	}
	return ""
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func formatDiscussion(e *gh.DiscussionEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	disc := e.GetDiscussion()
	action := e.GetAction()
	sender := e.GetSender().GetLogin()

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Discussion #%d %s</b>\n", disc.GetNumber(), esc(action))
	fmt.Fprintf(&b, "%s · %s\n", repoLink(owner, name), esc(disc.GetTitle()))
	fmt.Fprintf(&b, "by %s\n", userLink(sender))
	if disc.GetHTMLURL() != "" {
		fmt.Fprintf(&b, `<a href="%s">Open discussion</a>`, esc(disc.GetHTMLURL()))
	}
	return b.String()
}

func formatDiscussionComment(e *gh.DiscussionCommentEvent) string {
	repo := e.GetRepo()
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	disc := e.GetDiscussion()
	comment := e.GetComment()
	user := comment.GetUser().GetLogin()
	body := truncate(comment.GetBody(), 500)

	var b strings.Builder
	fmt.Fprintf(&b, "<b>New comment</b> on discussion #%d\n", disc.GetNumber())
	fmt.Fprintf(&b, "%s · %s\n", repoLink(owner, name), esc(disc.GetTitle()))
	fmt.Fprintf(&b, "by %s\n", userLink(user))
	if body != "" {
		fmt.Fprintf(&b, "\n%s", esc(body))
	}
	return b.String()
}
