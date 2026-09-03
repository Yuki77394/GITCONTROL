// Package ghops contains higher-level GitHub operations built on top of the
// per-user client returned by ghaccess. Each sub-file in this package groups
// related operations (repos, issues, PRs, actions, releases, files,
// branches, commits, discussions, search).
//
// All operations accept a *github.Client so callers retain control over
// authentication. None of these functions log or return secrets.
package ghops

import (
	"errors"
	"fmt"

	"github.com/swaggymusic/github-bot/internal/validation"

	gh "github.com/google/go-github/v66/github"
)

// ErrNotFound is returned when a GitHub API call returns 404.
var ErrNotFound = errors.New("not found")

// ErrForbidden is returned when a GitHub API call returns 403.
var ErrForbidden = errors.New("forbidden (insufficient GitHub permissions)")

// ClassifyError maps a github.ErrorResponse into a stable sentinel error
// where possible.
func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	var er *gh.ErrorResponse
	if errors.As(err, &er) {
		if er.Response != nil {
			switch er.Response.StatusCode {
			case 404:
				return fmt.Errorf("%w: %s", ErrNotFound, errMessage(er))
			case 403:
				return fmt.Errorf("%w: %s", ErrForbidden, errMessage(er))
			}
		}
	}
	return err
}

func errMessage(er *gh.ErrorResponse) string {
	if er.Message != "" {
		return er.Message
	}
	if len(er.Errors) > 0 && er.Errors[0].Message != "" {
		return er.Errors[0].Message
	}
	return ""
}

// SplitRepo splits "owner/repo" into (owner, repo, error).
func SplitRepo(full string) (string, string, error) {
	return validation.ValidateRepoName(full)
}
