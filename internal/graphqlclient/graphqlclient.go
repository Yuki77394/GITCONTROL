// Package graphqlclient provides a thin GitHub GraphQL API client used for
// operations that the REST client (google/go-github) does not expose:
// convertPullRequestToDraft, markPullRequestReadyForReview, pinIssue,
// unpinIssue, and Discussions (list/create/mark-answer).
//
// The client uses github.com/shurcooL/graphql over the same oauth2.HTTPClient
// that the REST client uses, so authentication is unified.
//
// SECURITY: the token is stored in the Client struct but never logged. No
// method on Client returns the token.
package graphqlclient

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gql "github.com/shurcooL/graphql"
	"golang.org/x/oauth2"
)

// Client wraps a shurcooL/graphql client targeting the GitHub GraphQL endpoint.
//
// SECURITY: The token is NOT stored on this struct — it lives only inside the
// oauth2 HTTPClient transport. There is no accessor that returns the token.
type Client struct {
	gql      *gql.Client
	endpoint string
	// authenticated is a boolean flag (no value) indicating that the client
	// was constructed with a non-empty token. It exists so callers can
	// verify a client is authenticated without leaking the token value.
	authenticated bool
}

// DefaultEndpoint is the GitHub.com GraphQL API URL.
const DefaultEndpoint = "https://api.github.com/graphql"

// NewClient builds a GraphQL client authenticated with the given token.
// apiURL should be the REST API base (e.g. https://api.github.com or
// https://enterprise.example.com/api/v3); this function derives the GraphQL
// endpoint from it.
func NewClient(ctx context.Context, token, apiURL string) (*Client, error) {
	if token == "" {
		return nil, errors.New("graphqlclient: empty token")
	}
	endpoint := deriveGraphQLEndpoint(apiURL)
	src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, src)
	return &Client{
		gql:           gql.NewClient(endpoint, httpClient),
		endpoint:      endpoint,
		authenticated: true,
	}, nil
}

// Endpoint returns the GraphQL endpoint URL (for diagnostics).
func (c *Client) Endpoint() string { return c.endpoint }

// deriveGraphQLEndpoint converts a REST API base URL into the GraphQL endpoint.
//   - https://api.github.com       → https://api.github.com/graphql
//   - https://host/api/v3          → https://host/api/graphql
//   - https://host/api/v3/         → https://host/api/graphql
//   - anything else                → <base>/graphql  (best-effort)
func deriveGraphQLEndpoint(apiURL string) string {
	if apiURL == "" {
		return DefaultEndpoint
	}
	apiURL = strings.TrimRight(apiURL, "/")
	if apiURL == "https://api.github.com" {
		return DefaultEndpoint
	}
	if strings.HasSuffix(apiURL, "/api/v3") {
		return strings.TrimSuffix(apiURL, "/v3") + "/graphql"
	}
	return apiURL + "/graphql"
}

// ---------------------------------------------------------------------------
// Pull Request: convert to draft / mark ready for review
// ---------------------------------------------------------------------------

// ConvertPullRequestToDraft converts a PR to draft status via the
// `convertPullRequestToDraft` GraphQL mutation.
//
// Returns:
//   - (alreadyDraft=true, nil) if the PR was already a draft (no-op success)
//   - (alreadyDraft=false, nil) on successful conversion
//   - (false, err) on API error
func (c *Client) ConvertPullRequestToDraft(ctx context.Context, nodeID string) (alreadyDraft bool, err error) {
	if nodeID == "" {
		return false, errors.New("graphqlclient: empty nodeID")
	}
	var m struct {
		ConvertPullRequestToDraft struct {
			PullRequest struct {
				ID      gql.ID
				IsDraft bool
			}
		} `graphql:"convertPullRequestToDraft(input: {pullRequestId: $id})"`
	}
	vars := map[string]interface{}{"id": gql.ID(nodeID)}
	if err := c.gql.Mutate(ctx, &m, vars); err != nil {
		return false, fmt.Errorf("graphql: convertPullRequestToDraft: %w", err)
	}
	return m.ConvertPullRequestToDraft.PullRequest.IsDraft, nil
}

// MarkPullRequestReadyForReview marks a draft PR as ready for review via the
// `markPullRequestReadyForReview` GraphQL mutation.
//
// Returns:
//   - (wasDraft=true, nil) on successful conversion (PR is no longer draft)
//   - (wasDraft=false, nil) if the PR was not a draft (no-op success)
//   - (false, err) on API error
func (c *Client) MarkPullRequestReadyForReview(ctx context.Context, nodeID string) (wasDraft bool, err error) {
	if nodeID == "" {
		return false, errors.New("graphqlclient: empty nodeID")
	}
	var m struct {
		MarkPullRequestReadyForReview struct {
			PullRequest struct {
				ID      gql.ID
				IsDraft bool
			}
		} `graphql:"markPullRequestReadyForReview(input: {pullRequestId: $id})"`
	}
	vars := map[string]interface{}{"id": gql.ID(nodeID)}
	if err := c.gql.Mutate(ctx, &m, vars); err != nil {
		return false, fmt.Errorf("graphql: markPullRequestReadyForReview: %w", err)
	}
	// If the PR is still a draft after the mutation, it was already ready
	// (shouldn't happen, but we treat it as "was draft=false" to be safe).
	return !m.MarkPullRequestReadyForReview.PullRequest.IsDraft, nil
}

// ---------------------------------------------------------------------------
// Issue: pin / unpin
// ---------------------------------------------------------------------------

// PinIssue pins an issue via the `pinIssue` GraphQL mutation.
func (c *Client) PinIssue(ctx context.Context, nodeID string) error {
	if nodeID == "" {
		return errors.New("graphqlclient: empty nodeID")
	}
	var m struct {
		PinIssue struct {
			Issue struct {
				ID gql.ID
			}
		} `graphql:"pinIssue(input: {issueId: $id})"`
	}
	vars := map[string]interface{}{"id": gql.ID(nodeID)}
	if err := c.gql.Mutate(ctx, &m, vars); err != nil {
		return fmt.Errorf("graphql: pinIssue: %w", err)
	}
	return nil
}

// UnpinIssue unpins an issue via the `unpinIssue` GraphQL mutation.
func (c *Client) UnpinIssue(ctx context.Context, nodeID string) error {
	if nodeID == "" {
		return errors.New("graphqlclient: empty nodeID")
	}
	var m struct {
		UnpinIssue struct {
			Issue struct {
				ID gql.ID
			}
		} `graphql:"unpinIssue(input: {issueId: $id})"`
	}
	vars := map[string]interface{}{"id": gql.ID(nodeID)}
	if err := c.gql.Mutate(ctx, &m, vars); err != nil {
		return fmt.Errorf("graphql: unpinIssue: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Discussions
// ---------------------------------------------------------------------------

// Discussion is a minimal GitHub Discussion representation.
type Discussion struct {
	Number     int32
	Title      string
	Body       string
	URL        string
	IsAnswered bool
	Author     string
}

// DiscussionCategory represents a GitHub Discussion category.
type DiscussionCategory struct {
	ID    string
	Name  string
	Emoji string
}

// ListDiscussions returns discussions for a repository (by owner/name).
// perPage must be 1..100; we cap to 25 to keep Telegram output sane.
// Only page=1 is supported (GitHub GraphQL uses cursor-based pagination;
// callers can pass a larger perPage to fetch more).
func (c *Client) ListDiscussions(ctx context.Context, owner, repo string, perPage int) ([]Discussion, error) {
	if perPage <= 0 || perPage > 25 {
		perPage = 10
	}
	var q struct {
		Repository struct {
			Discussions struct {
				Nodes []struct {
					Number     int32
					Title      string
					Body       string
					URL        string `graphql:"url"`
					IsAnswered bool   `graphql:"isAnswered"`
					Author     struct {
						Login string
					}
				}
			} `graphql:"discussions(first: $perPage)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}
	vars := map[string]interface{}{
		"owner":   gql.String(owner),
		"repo":    gql.String(repo),
		"perPage": gql.Int(perPage),
	}
	if err := c.gql.Query(ctx, &q, vars); err != nil {
		return nil, fmt.Errorf("graphql: discussions query: %w", err)
	}
	out := make([]Discussion, 0, len(q.Repository.Discussions.Nodes))
	for _, n := range q.Repository.Discussions.Nodes {
		out = append(out, Discussion{
			Number:     n.Number,
			Title:      n.Title,
			Body:       n.Body,
			URL:        n.URL,
			IsAnswered: n.IsAnswered,
			Author:     n.Author.Login,
		})
	}
	return out, nil
}

// ListDiscussionCategories returns the discussion categories available on
// a repository. Used by CreateDiscussion to resolve a category name to an ID.
func (c *Client) ListDiscussionCategories(ctx context.Context, owner, repo string) ([]DiscussionCategory, error) {
	var q struct {
		Repository struct {
			DiscussionCategories struct {
				Nodes []struct {
					ID    gql.ID
					Name  string
					Emoji string
				}
			} `graphql:"discussionCategories(first: 50)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}
	vars := map[string]interface{}{
		"owner": gql.String(owner),
		"repo":  gql.String(repo),
	}
	if err := c.gql.Query(ctx, &q, vars); err != nil {
		return nil, fmt.Errorf("graphql: discussion categories: %w", err)
	}
	out := make([]DiscussionCategory, 0, len(q.Repository.DiscussionCategories.Nodes))
	for _, n := range q.Repository.DiscussionCategories.Nodes {
		out = append(out, DiscussionCategory{
			ID:    n.ID.(gql.ID).(string),
			Name:  n.Name,
			Emoji: n.Emoji,
		})
	}
	return out, nil
}

// GetRepoNodeID resolves the GraphQL Node ID of a repository by owner/name.
// Needed as input to CreateDiscussion.
func (c *Client) GetRepoNodeID(ctx context.Context, owner, repo string) (string, error) {
	var q struct {
		Repository struct {
			ID gql.ID `graphql:"id"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}
	vars := map[string]interface{}{
		"owner": gql.String(owner),
		"repo":  gql.String(repo),
	}
	if err := c.gql.Query(ctx, &q, vars); err != nil {
		return "", fmt.Errorf("graphql: repository id: %w", err)
	}
	return q.Repository.ID.(gql.ID).(string), nil
}

// CreateDiscussion creates a new discussion.
// discussionCategoryID is required by GitHub; callers must first fetch the
// repository's discussion categories via ListDiscussionCategories.
func (c *Client) CreateDiscussion(ctx context.Context, repoNodeID, discussionCategoryID, title, body string) (string, error) {
	if repoNodeID == "" || discussionCategoryID == "" || title == "" {
		return "", errors.New("graphqlclient: repoNodeID, discussionCategoryID, and title are required")
	}
	var m struct {
		CreateDiscussion struct {
			Discussion struct {
				Number int32
				URL    string `graphql:"url"`
			}
		} `graphql:"createDiscussion(input: {repositoryId: $repoId, categoryId: $catId, title: $title, body: $body})"`
	}
	vars := map[string]interface{}{
		"repoId": gql.ID(repoNodeID),
		"catId":  gql.ID(discussionCategoryID),
		"title":  gql.String(title),
		"body":   gql.String(body),
	}
	if err := c.gql.Mutate(ctx, &m, vars); err != nil {
		return "", fmt.Errorf("graphql: createDiscussion: %w", err)
	}
	return m.CreateDiscussion.Discussion.URL, nil
}

// MarkDiscussionCommentAsAnswer marks a discussion comment as the answer.
// commentNodeID is the GraphQL Node ID of the comment to mark.
func (c *Client) MarkDiscussionCommentAsAnswer(ctx context.Context, commentNodeID string) error {
	if commentNodeID == "" {
		return errors.New("graphqlclient: empty commentNodeID")
	}
	var m struct {
		MarkDiscussionCommentAsAnswer struct {
			ClientMutationID *string
		} `graphql:"markDiscussionCommentAsAnswer(input: {id: $id})"`
	}
	vars := map[string]interface{}{"id": gql.ID(commentNodeID)}
	if err := c.gql.Mutate(ctx, &m, vars); err != nil {
		return fmt.Errorf("graphql: markDiscussionCommentAsAnswer: %w", err)
	}
	return nil
}

// LookupDiscussionCommentNodeID finds the GraphQL Node ID of a discussion
// comment by its REST database ID. Used by /answered to resolve a replied-to
// discussion comment.
func (c *Client) LookupDiscussionCommentNodeID(ctx context.Context, owner, repo string, discussionNumber int, restCommentID int64) (string, error) {
	var q struct {
		Repository struct {
			Discussion struct {
				Comments struct {
					Nodes []struct {
						DatabaseID int64 `graphql:"databaseId"`
						ID         gql.ID
					}
				} `graphql:"comments(first: 100)"`
			} `graphql:"discussion(number: $num)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}
	vars := map[string]interface{}{
		"owner": gql.String(owner),
		"repo":  gql.String(repo),
		"num":   gql.Int(discussionNumber),
	}
	if err := c.gql.Query(ctx, &q, vars); err != nil {
		return "", fmt.Errorf("graphql: discussion comments: %w", err)
	}
	for _, n := range q.Repository.Discussion.Comments.Nodes {
		if n.DatabaseID == restCommentID {
			return n.ID.(gql.ID).(string), nil
		}
	}
	return "", fmt.Errorf("graphqlclient: discussion comment with databaseId %d not found in discussion #%d", restCommentID, discussionNumber)
}

// DeriveGraphQLEndpoint is exported for tests.
func DeriveGraphQLEndpoint(apiURL string) string {
	return deriveGraphQLEndpoint(apiURL)
}
