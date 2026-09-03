package ghops

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v66/github"
)

// CreateIssue opens a new issue with title and optional body.
func CreateIssue(ctx context.Context, c *gh.Client, owner, repo, title, body string, labels, assignees []string) (*gh.Issue, error) {
	req := &gh.IssueRequest{
		Title:     gh.String(title),
		Body:      gh.String(body),
		Labels:    &labels,
		Assignees: &assignees,
	}
	issue, _, err := c.Issues.Create(ctx, owner, repo, req)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return issue, nil
}

// GetIssue returns a single issue by number.
func GetIssue(ctx context.Context, c *gh.Client, owner, repo string, number int) (*gh.Issue, error) {
	i, _, err := c.Issues.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return i, nil
}

// ListIssues returns open issues in the repo.
func ListIssues(ctx context.Context, c *gh.Client, owner, repo string, page, perPage int, state string) ([]*gh.Issue, error) {
	if state == "" {
		state = "open"
	}
	if perPage <= 0 {
		perPage = 10
	}
	opts := &gh.IssueListByRepoOptions{
		State:       state,
		ListOptions: gh.ListOptions{Page: page, PerPage: perPage},
	}
	list, _, err := c.Issues.ListByRepo(ctx, owner, repo, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list, nil
}

// CloseIssue closes an issue or PR.
func CloseIssue(ctx context.Context, c *gh.Client, owner, repo string, number int) error {
	return updateIssueState(ctx, c, owner, repo, number, "closed")
}

// ReopenIssue reopens an issue or PR.
func ReopenIssue(ctx context.Context, c *gh.Client, owner, repo string, number int) error {
	return updateIssueState(ctx, c, owner, repo, number, "open")
}

func updateIssueState(ctx context.Context, c *gh.Client, owner, repo string, number int, state string) error {
	req := &gh.IssueRequest{State: gh.String(state)}
	_, _, err := c.Issues.Edit(ctx, owner, repo, number, req)
	return ClassifyError(err)
}

// CommentIssue adds a comment to an issue or PR.
func CommentIssue(ctx context.Context, c *gh.Client, owner, repo string, number int, body string) (*gh.IssueComment, error) {
	comment, _, err := c.Issues.CreateComment(ctx, owner, repo, number, &gh.IssueComment{Body: gh.String(body)})
	if err != nil {
		return nil, ClassifyError(err)
	}
	return comment, nil
}

// AssignUsers adds assignees to an issue/PR.
func AssignUsers(ctx context.Context, c *gh.Client, owner, repo string, number int, assignees []string) error {
	_, _, err := c.Issues.AddAssignees(ctx, owner, repo, number, assignees)
	return ClassifyError(err)
}

// UnassignUsers removes assignees.
func UnassignUsers(ctx context.Context, c *gh.Client, owner, repo string, number int, assignees []string) error {
	_, _, err := c.Issues.RemoveAssignees(ctx, owner, repo, number, assignees)
	return ClassifyError(err)
}

// AddLabels adds labels to an issue/PR.
func AddLabels(ctx context.Context, c *gh.Client, owner, repo string, number int, labels []string) error {
	_, _, err := c.Issues.AddLabelsToIssue(ctx, owner, repo, number, labels)
	return ClassifyError(err)
}

// RemoveLabel removes a label from an issue/PR.
func RemoveLabel(ctx context.Context, c *gh.Client, owner, repo string, number int, label string) error {
	_, err := c.Issues.RemoveLabelForIssue(ctx, owner, repo, number, label)
	return ClassifyError(err)
}

// ListLabels lists labels defined on the repo.
func ListLabels(ctx context.Context, c *gh.Client, owner, repo string, page, perPage int) ([]*gh.Label, error) {
	if perPage <= 0 {
		perPage = 20
	}
	opts := &gh.ListOptions{Page: page, PerPage: perPage}
	list, _, err := c.Issues.ListLabels(ctx, owner, repo, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list, nil
}

// LockIssue locks conversation.
func LockIssue(ctx context.Context, c *gh.Client, owner, repo string, number int, reason string) error {
	opts := &gh.LockIssueOptions{LockReason: reason}
	_, err := c.Issues.Lock(ctx, owner, repo, number, opts)
	return ClassifyError(err)
}

// UnlockIssue unlocks conversation.
func UnlockIssue(ctx context.Context, c *gh.Client, owner, repo string, number int) error {
	_, err := c.Issues.Unlock(ctx, owner, repo, number)
	return ClassifyError(err)
}

// PinIssue pins an issue using the GitHub GraphQL `pinIssue` mutation.
// gqc is a GraphQL client; if nil, returns ErrUnsupported.
//
// Returns nil on success. If the issue is already pinned, GitHub returns
// the issue unchanged (no error); we treat this as success.
func PinIssue(ctx context.Context, c *gh.Client, gqc PinIssueClient, owner, repo string, number int) error {
	if gqc == nil {
		return ErrUnsupported
	}
	issue, err := GetIssue(ctx, c, owner, repo, number)
	if err != nil {
		return err
	}
	nodeID := issue.GetNodeID()
	if nodeID == "" {
		return fmt.Errorf("ghops: issue has no NodeID (GitHub API did not return one)")
	}
	return gqc.PinIssue(ctx, nodeID)
}

// UnpinIssue unpins an issue using the GitHub GraphQL `unpinIssue` mutation.
// gqc is a GraphQL client; if nil, returns ErrUnsupported.
func UnpinIssue(ctx context.Context, c *gh.Client, gqc UnpinIssueClient, owner, repo string, number int) error {
	if gqc == nil {
		return ErrUnsupported
	}
	issue, err := GetIssue(ctx, c, owner, repo, number)
	if err != nil {
		return err
	}
	nodeID := issue.GetNodeID()
	if nodeID == "" {
		return fmt.Errorf("ghops: issue has no NodeID (GitHub API did not return one)")
	}
	return gqc.UnpinIssue(ctx, nodeID)
}

// PinIssueClient is the subset of graphqlclient.Client used by PinIssue.
type PinIssueClient interface {
	PinIssue(ctx context.Context, nodeID string) error
}

// UnpinIssueClient is the subset of graphqlclient.Client used by UnpinIssue.
type UnpinIssueClient interface {
	UnpinIssue(ctx context.Context, nodeID string) error
}

// ErrUnsupported indicates an operation not supported by the current
// go-github client version (e.g. GraphQL-only endpoints).
var ErrUnsupported = errUnsupported{}

type errUnsupported struct{}

func (errUnsupported) Error() string { return "operation not supported by current API client" }

// SetMilestone sets the milestone for an issue/PR.
func SetMilestone(ctx context.Context, c *gh.Client, owner, repo string, number int, milestoneNumber int) error {
	req := &gh.IssueRequest{Milestone: gh.Int(milestoneNumber)}
	_, _, err := c.Issues.Edit(ctx, owner, repo, number, req)
	return ClassifyError(err)
}
