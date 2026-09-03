package ghops

import (
	"context"

	gh "github.com/google/go-github/v66/github"
)

// ListReleases returns the most recent releases.
func ListReleases(ctx context.Context, c *gh.Client, owner, repo string, page, perPage int) ([]*gh.RepositoryRelease, error) {
	if perPage <= 0 {
		perPage = 10
	}
	list, _, err := c.Repositories.ListReleases(ctx, owner, repo, &gh.ListOptions{Page: page, PerPage: perPage})
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list, nil
}

// GetLatestRelease returns the latest non-prerelease release.
func GetLatestRelease(ctx context.Context, c *gh.Client, owner, repo string) (*gh.RepositoryRelease, error) {
	r, _, err := c.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return r, nil
}

// CreateRelease creates a new release. If generateReleaseNotes is true,
// GitHub auto-generates the body from commits.
func CreateRelease(ctx context.Context, c *gh.Client, owner, repo, tag, name, body, target string, draft, prerelease, generateNotes bool) (*gh.RepositoryRelease, error) {
	req := &gh.RepositoryRelease{
		TagName:              gh.String(tag),
		Name:                 gh.String(name),
		Body:                 gh.String(body),
		TargetCommitish:      gh.String(target),
		Draft:                gh.Bool(draft),
		Prerelease:           gh.Bool(prerelease),
		GenerateReleaseNotes: gh.Bool(generateNotes),
	}
	r, _, err := c.Repositories.CreateRelease(ctx, owner, repo, req)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return r, nil
}

// GenerateReleaseNotes asks GitHub to generate release notes for a tag.
func GenerateReleaseNotes(ctx context.Context, c *gh.Client, owner, repo, tag, previousTag string) (string, error) {
	req := &gh.GenerateNotesOptions{
		TagName:         tag,
		PreviousTagName: gh.String(previousTag),
	}
	out, _, err := c.Repositories.GenerateReleaseNotes(ctx, owner, repo, req)
	if err != nil {
		return "", ClassifyError(err)
	}
	return out.Body, nil
}
