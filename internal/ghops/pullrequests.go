package ghops

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v66/github"
)

// ListPRs returns pull requests in the repo.
func ListPRs(ctx context.Context, c *gh.Client, owner, repo, state string, page, perPage int) ([]*gh.PullRequest, error) {
	if state == "" {
		state = "open"
	}
	if perPage <= 0 {
		perPage = 10
	}
	opts := &gh.PullRequestListOptions{
		State:       state,
		ListOptions: gh.ListOptions{Page: page, PerPage: perPage},
	}
	list, _, err := c.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list, nil
}

// GetPR returns a single PR by number.
func GetPR(ctx context.Context, c *gh.Client, owner, repo string, number int) (*gh.PullRequest, error) {
	pr, _, err := c.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return pr, nil
}

// ListPRCommits returns the commits on a PR.
func ListPRCommits(ctx context.Context, c *gh.Client, owner, repo string, number int) ([]*gh.RepositoryCommit, error) {
	list, _, err := c.PullRequests.ListCommits(ctx, owner, repo, number, nil)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list, nil
}

// ListPRFiles returns the files changed in a PR.
func ListPRFiles(ctx context.Context, c *gh.Client, owner, repo string, number int) ([]*gh.CommitFile, error) {
	list, _, err := c.PullRequests.ListFiles(ctx, owner, repo, number, nil)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list, nil
}

// ListPRReviews returns review history.
func ListPRReviews(ctx context.Context, c *gh.Client, owner, repo string, number int) ([]*gh.PullRequestReview, error) {
	list, _, err := c.PullRequests.ListReviews(ctx, owner, repo, number, nil)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list, nil
}

// ApprovePR submits an approval review.
func ApprovePR(ctx context.Context, c *gh.Client, owner, repo string, number int, body string) error {
	_, _, err := c.PullRequests.CreateReview(ctx, owner, repo, number, &gh.PullRequestReviewRequest{
		Event: gh.String("APPROVE"),
		Body:  gh.String(body),
	})
	return ClassifyError(err)
}

// RequestChanges submits a "request changes" review.
func RequestChanges(ctx context.Context, c *gh.Client, owner, repo string, number int, body string) error {
	_, _, err := c.PullRequests.CreateReview(ctx, owner, repo, number, &gh.PullRequestReviewRequest{
		Event: gh.String("REQUEST_CHANGES"),
		Body:  gh.String(body),
	})
	return ClassifyError(err)
}

// MergeMethod enumerates supported merge strategies.
type MergeMethod string

const (
	MergeMethodMerge  MergeMethod = "merge"
	MergeMethodSquash MergeMethod = "squash"
	MergeMethodRebase MergeMethod = "rebase"
)

// MergePR merges a PR with the given method. Returns an error if the PR is
// not mergeable or the user lacks permission.
func MergePR(ctx context.Context, c *gh.Client, owner, repo string, number int, method MergeMethod, commitTitle, commitMessage string) error {
	m := string(method)
	_, _, err := c.PullRequests.Merge(ctx, owner, repo, number, commitMessage, &gh.PullRequestOptions{
		MergeMethod: m,
		CommitTitle: commitTitle,
	})
	return ClassifyError(err)
}

// ConvertPRToDraft converts a PR to draft status using the GitHub GraphQL API.
// gqc is a GraphQL client authenticated as the user; if gqc is nil, returns
// ErrUnsupported. The PR's GraphQL Node ID is resolved from owner/repo/number
// via the REST API (pr.GetNodeID()).
//
// Returns nil on success, including the no-op case where the PR was already
// a draft.
func ConvertPRToDraft(ctx context.Context, c *gh.Client, gqc ConvertDraftClient, owner, repo string, number int) error {
	if gqc == nil {
		return ErrUnsupported
	}
	pr, err := GetPR(ctx, c, owner, repo, number)
	if err != nil {
		return err
	}
	if pr.GetDraft() {
		return nil // already draft
	}
	nodeID := pr.GetNodeID()
	if nodeID == "" {
		return fmt.Errorf("ghops: PR has no NodeID (GitHub API did not return one)")
	}
	_, err = gqc.ConvertPullRequestToDraft(ctx, nodeID)
	return err
}

// MarkPRReady marks a draft PR as ready for review using the GitHub GraphQL API.
// gqc is a GraphQL client; if nil, returns ErrUnsupported.
//
// Returns nil on success, including the no-op case where the PR was not a
// draft.
func MarkPRReady(ctx context.Context, c *gh.Client, gqc MarkReadyClient, owner, repo string, number int) error {
	if gqc == nil {
		return ErrUnsupported
	}
	pr, err := GetPR(ctx, c, owner, repo, number)
	if err != nil {
		return err
	}
	if !pr.GetDraft() {
		return nil // already ready
	}
	nodeID := pr.GetNodeID()
	if nodeID == "" {
		return fmt.Errorf("ghops: PR has no NodeID (GitHub API did not return one)")
	}
	_, err = gqc.MarkPullRequestReadyForReview(ctx, nodeID)
	return err
}

// ConvertDraftClient is the subset of graphqlclient.Client used by
// ConvertPRToDraft. Extracted as an interface for testability.
type ConvertDraftClient interface {
	ConvertPullRequestToDraft(ctx context.Context, nodeID string) (alreadyDraft bool, err error)
}

// MarkReadyClient is the subset of graphqlclient.Client used by MarkPRReady.
type MarkReadyClient interface {
	MarkPullRequestReadyForReview(ctx context.Context, nodeID string) (wasDraft bool, err error)
}

// RequestReviewers requests reviewers on a PR.
func RequestReviewers(ctx context.Context, c *gh.Client, owner, repo string, number int, reviewers []string) error {
	_, _, err := c.PullRequests.RequestReviewers(ctx, owner, repo, number, gh.ReviewersRequest{
		Reviewers: reviewers,
	})
	return ClassifyError(err)
}

// ListChecks returns check runs for the PR head SHA.
func ListChecks(ctx context.Context, c *gh.Client, owner, repo, ref string) ([]*gh.CheckRun, error) {
	list, _, err := c.Checks.ListCheckRunsForRef(ctx, owner, repo, ref, nil)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list.CheckRuns, nil
}

// PRMergeable returns true if the PR is currently mergeable.
func PRMergeable(ctx context.Context, c *gh.Client, owner, repo string, number int) (bool, error) {
	pr, err := GetPR(ctx, c, owner, repo, number)
	if err != nil {
		return false, err
	}
	if pr.GetMergeable() {
		return true, nil
	}
	return false, fmt.Errorf("PR is not mergeable (state=%s, mergeable_state=%s)", pr.GetState(), pr.GetMergeableState())
}
