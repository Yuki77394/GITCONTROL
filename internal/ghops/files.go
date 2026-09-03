package ghops

import (
	"context"
	"fmt"

	"github.com/swaggymusic/github-bot/internal/validation"

	gh "github.com/google/go-github/v66/github"
)

// GetFileContent fetches a file's metadata and (for text files small enough)
// its decoded content. path may be "" for the repo root listing.
func GetFileContent(ctx context.Context, c *gh.Client, owner, repo, branch, path string) (*gh.RepositoryContent, []byte, error) {
	if err := validation.ValidateFilePath(path); err != nil {
		return nil, nil, err
	}
	opts := &gh.RepositoryContentGetOptions{Ref: branch}
	file, dir, _, err := c.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		return nil, nil, ClassifyError(err)
	}
	if dir != nil {
		// Caller asked for a file but got a directory.
		return nil, nil, fmt.Errorf("ghops: path is a directory")
	}
	if file == nil {
		return nil, nil, fmt.Errorf("ghops: file not found")
	}
	content, err := file.GetContent()
	if err != nil {
		return file, nil, fmt.Errorf("ghops: decode content: %w", err)
	}
	return file, []byte(content), nil
}

// ListDir returns the entries of a directory at the given path (or root if "").
func ListDir(ctx context.Context, c *gh.Client, owner, repo, branch, path string) ([]*gh.RepositoryContent, error) {
	if err := validation.ValidateFilePath(path); err != nil {
		return nil, err
	}
	opts := &gh.RepositoryContentGetOptions{Ref: branch}
	_, dir, _, err := c.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	if dir == nil {
		return nil, fmt.Errorf("ghops: path is a file, not a directory")
	}
	return dir, nil
}

// CreateFile creates a new file in the repo.
func CreateFile(ctx context.Context, c *gh.Client, owner, repo, branch, path, message, content string) error {
	if err := validation.ValidateFilePath(path); err != nil {
		return err
	}
	opts := &gh.RepositoryContentFileOptions{
		Message: gh.String(message),
		Content: []byte(content),
		Branch:  gh.String(branch),
	}
	_, _, err := c.Repositories.CreateFile(ctx, owner, repo, path, opts)
	return ClassifyError(err)
}

// UpdateFile updates an existing file. Requires the current blob SHA.
func UpdateFile(ctx context.Context, c *gh.Client, owner, repo, branch, path, message, content, sha string) error {
	if err := validation.ValidateFilePath(path); err != nil {
		return err
	}
	opts := &gh.RepositoryContentFileOptions{
		Message: gh.String(message),
		Content: []byte(content),
		SHA:     gh.String(sha),
		Branch:  gh.String(branch),
	}
	_, _, err := c.Repositories.UpdateFile(ctx, owner, repo, path, opts)
	return ClassifyError(err)
}

// DeleteFile removes a file.
func DeleteFile(ctx context.Context, c *gh.Client, owner, repo, branch, path, message, sha string) error {
	if err := validation.ValidateFilePath(path); err != nil {
		return err
	}
	opts := &gh.RepositoryContentFileOptions{
		Message: gh.String(message),
		SHA:     gh.String(sha),
		Branch:  gh.String(branch),
	}
	_, _, err := c.Repositories.DeleteFile(ctx, owner, repo, path, opts)
	return ClassifyError(err)
}
