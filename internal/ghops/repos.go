package ghops

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v66/github"
)

// ListUserRepos returns a page of repositories accessible to the user.
func ListUserRepos(ctx context.Context, c *gh.Client, page, perPage int) ([]*gh.Repository, *gh.Response, error) {
	opts := &gh.RepositoryListOptions{
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: gh.ListOptions{Page: page, PerPage: perPage},
	}
	repos, resp, err := c.Repositories.List(ctx, "", opts)
	if err != nil {
		return nil, nil, ClassifyError(err)
	}
	return repos, resp, nil
}

// GetRepo returns metadata for owner/repo.
func GetRepo(ctx context.Context, c *gh.Client, owner, repo string) (*gh.Repository, error) {
	r, _, err := c.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return r, nil
}

// Star marks a repo as starred by the authenticated user.
func Star(ctx context.Context, c *gh.Client, owner, repo string) error {
	_, err := c.Activity.Star(ctx, owner, repo)
	return ClassifyError(err)
}

// Unstar removes the user's star.
func Unstar(ctx context.Context, c *gh.Client, owner, repo string) error {
	_, err := c.Activity.Unstar(ctx, owner, repo)
	return ClassifyError(err)
}

// Watch subscribes the user to repo notifications.
func Watch(ctx context.Context, c *gh.Client, owner, repo, sub string) error {
	_, _, err := c.Activity.SetRepositorySubscription(ctx, owner, repo, &gh.Subscription{Subscribed: gh.Bool(true), Ignored: gh.Bool(false)})
	return ClassifyError(err)
}

// Unwatch removes the user's subscription.
func Unwatch(ctx context.Context, c *gh.Client, owner, repo string) error {
	_, err := c.Activity.DeleteRepositorySubscription(ctx, owner, repo)
	return ClassifyError(err)
}

// Fork creates a fork of the repo in the user's account.
func Fork(ctx context.Context, c *gh.Client, owner, repo string) (*gh.Repository, error) {
	f, _, err := c.Repositories.CreateFork(ctx, owner, repo, &gh.RepositoryCreateForkOptions{})
	if err != nil {
		return nil, ClassifyError(err)
	}
	return f, nil
}

// Archive marks the repo as archived (read-only). Requires admin permission.
func Archive(ctx context.Context, c *gh.Client, owner, repo string) error {
	_, _, err := c.Repositories.Edit(ctx, owner, repo, &gh.Repository{Archived: gh.Bool(true)})
	return ClassifyError(err)
}

// Unarchive reverses Archive.
func Unarchive(ctx context.Context, c *gh.Client, owner, repo string) error {
	_, _, err := c.Repositories.Edit(ctx, owner, repo, &gh.Repository{Archived: gh.Bool(false)})
	return ClassifyError(err)
}

// ListContributors returns the top contributors.
func ListContributors(ctx context.Context, c *gh.Client, owner, repo string, limit int) ([]*gh.Contributor, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	opts := &gh.ListContributorsOptions{ListOptions: gh.ListOptions{PerPage: limit}}
	list, _, err := c.Repositories.ListContributors(ctx, owner, repo, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list, nil
}

// ListLanguages returns the language breakdown.
func ListLanguages(ctx context.Context, c *gh.Client, owner, repo string) (map[string]int, error) {
	langs, _, err := c.Repositories.ListLanguages(ctx, owner, repo)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return langs, nil
}

// CreateWebhook creates a repository webhook with the given URL, secret,
// and event list. Returns the created hook ID.
func CreateWebhook(ctx context.Context, c *gh.Client, owner, repo, url, secret string, events []string) (int64, error) {
	if url == "" {
		return 0, fmt.Errorf("ghops: webhook URL is empty")
	}
	contentType := "json"
	hook := &gh.Hook{
		Name:   gh.String("web"),
		Events: events,
		Config: &gh.HookConfig{
			URL:         gh.String(url),
			ContentType: &contentType,
			Secret:      gh.String(secret),
		},
		Active: gh.Bool(true),
	}
	created, _, err := c.Repositories.CreateHook(ctx, owner, repo, hook)
	if err != nil {
		return 0, ClassifyError(err)
	}
	return created.GetID(), nil
}

// DeleteWebhook removes a webhook by ID.
func DeleteWebhook(ctx context.Context, c *gh.Client, owner, repo string, hookID int64) error {
	_, err := c.Repositories.DeleteHook(ctx, owner, repo, hookID)
	return ClassifyError(err)
}
