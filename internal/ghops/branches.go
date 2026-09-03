package ghops

import (
	"context"

	gh "github.com/google/go-github/v66/github"
)

// ListBranches returns branches of a repo.
func ListBranches(ctx context.Context, c *gh.Client, owner, repo string, page, perPage int) ([]*gh.Branch, *gh.Response, error) {
	if perPage <= 0 {
		perPage = 10
	}
	opts := &gh.BranchListOptions{ListOptions: gh.ListOptions{Page: page, PerPage: perPage}}
	list, resp, err := c.Repositories.ListBranches(ctx, owner, repo, opts)
	if err != nil {
		return nil, nil, ClassifyError(err)
	}
	return list, resp, nil
}

// GetBranch returns a single branch by name.
func GetBranch(ctx context.Context, c *gh.Client, owner, repo, branch string) (*gh.Branch, error) {
	b, _, err := c.Repositories.GetBranch(ctx, owner, repo, branch, 0)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return b, nil
}

// CreateBranch creates a new branch from a source SHA.
func CreateBranch(ctx context.Context, c *gh.Client, owner, repo, newBranch, fromBranch string) error {
	// Resolve source SHA.
	src, _, err := c.Repositories.GetBranch(ctx, owner, repo, fromBranch, 0)
	if err != nil {
		return ClassifyError(err)
	}
	sha := src.GetCommit().GetSHA()
	_, _, err = c.Git.CreateRef(ctx, owner, repo, &gh.Reference{
		Ref: gh.String("refs/heads/" + newBranch),
		Object: &gh.GitObject{
			SHA: &sha,
		},
	})
	return ClassifyError(err)
}

// DeleteBranch removes a branch by name.
func DeleteBranch(ctx context.Context, c *gh.Client, owner, repo, branch string) error {
	_, err := c.Git.DeleteRef(ctx, owner, repo, "refs/heads/"+branch)
	return ClassifyError(err)
}

// SetDefaultBranch changes the default branch. Requires admin permission.
func SetDefaultBranch(ctx context.Context, c *gh.Client, owner, repo, branch string) error {
	_, _, err := c.Repositories.Edit(ctx, owner, repo, &gh.Repository{DefaultBranch: gh.String(branch)})
	return ClassifyError(err)
}
