package ghops

import (
	"context"

	gh "github.com/google/go-github/v66/github"
)

// SearchIssues searches issues within a repo.
func SearchIssues(ctx context.Context, c *gh.Client, owner, repo, query string, page, perPage int) (*gh.IssuesSearchResult, error) {
	if perPage <= 0 {
		perPage = 10
	}
	q := "repo:" + owner + "/" + repo + " is:issue " + query
	opts := &gh.SearchOptions{ListOptions: gh.ListOptions{Page: page, PerPage: perPage}}
	res, _, err := c.Search.Issues(ctx, q, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return res, nil
}

// SearchPRs searches pull requests within a repo.
func SearchPRs(ctx context.Context, c *gh.Client, owner, repo, query string, page, perPage int) (*gh.IssuesSearchResult, error) {
	if perPage <= 0 {
		perPage = 10
	}
	q := "repo:" + owner + "/" + repo + " is:pr " + query
	opts := &gh.SearchOptions{ListOptions: gh.ListOptions{Page: page, PerPage: perPage}}
	res, _, err := c.Search.Issues(ctx, q, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return res, nil
}

// SearchCode searches code within a repo. Requires the user's token to
// have the `code` search permission scope.
func SearchCode(ctx context.Context, c *gh.Client, owner, repo, query string, page, perPage int) (*gh.CodeSearchResult, error) {
	if perPage <= 0 {
		perPage = 10
	}
	q := "repo:" + owner + "/" + repo + " " + query
	opts := &gh.SearchOptions{ListOptions: gh.ListOptions{Page: page, PerPage: perPage}}
	res, _, err := c.Search.Code(ctx, q, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return res, nil
}
