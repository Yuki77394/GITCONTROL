package ghops

import (
	"context"

	gh "github.com/google/go-github/v66/github"
)

// GetCommit returns metadata for a single commit by SHA.
func GetCommit(ctx context.Context, c *gh.Client, owner, repo, sha string) (*gh.RepositoryCommit, error) {
	commit, _, err := c.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return commit, nil
}

// ListCommits returns recent commits on a branch.
func ListCommits(ctx context.Context, c *gh.Client, owner, repo, branch string, page, perPage int) ([]*gh.RepositoryCommit, error) {
	if perPage <= 0 {
		perPage = 10
	}
	opts := &gh.CommitsListOptions{
		SHA:         branch,
		ListOptions: gh.ListOptions{Page: page, PerPage: perPage},
	}
	list, _, err := c.Repositories.ListCommits(ctx, owner, repo, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list, nil
}

// Compare returns a comparison between two refs (branches, tags, or SHAs).
func Compare(ctx context.Context, c *gh.Client, owner, repo, base, head string) (*gh.CommitsComparison, error) {
	cmp, _, err := c.Repositories.CompareCommits(ctx, owner, repo, base, head, nil)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return cmp, nil
}
