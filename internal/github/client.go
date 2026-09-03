// Package github provides the SWAGGYMUSIC GitHub API client factory, OAuth
// helpers, webhook signature verification, and event type metadata.
//
// The package uses google/go-github (the widely-used community client) and
// golang.org/x/oauth2. It deliberately avoids any first-party dependency on
// the original reference repository author's packages.
package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gh "github.com/google/go-github/v66/github"
	"github.com/swaggymusic/github-bot/internal/encryption"

	"golang.org/x/oauth2"
)

// AuthMethod enumerates supported GitHub authentication methods.
type AuthMethod string

const (
	AuthMethodOAuth AuthMethod = "oauth"
	AuthMethodPAT   AuthMethod = "pat"
)

// ClientFactory builds *gh.Client instances authenticated per-user.
type ClientFactory struct {
	// DefaultAPIURL is used when an account does not specify its own.
	// Typically https://api.github.com.
	DefaultAPIURL string
}

// NewClientFactory creates a factory using the given default API URL.
func NewClientFactory(defaultAPIURL string) *ClientFactory {
	if defaultAPIURL == "" {
		defaultAPIURL = "https://api.github.com"
	}
	return &ClientFactory{DefaultAPIURL: defaultAPIURL}
}

// NewUserClient returns a GitHub client authenticated with the given OAuth/PAT
// token, talking to the given API URL (use "" for api.github.com).
func (f *ClientFactory) NewUserClient(ctx context.Context, token, apiURL string) (*gh.Client, error) {
	if token == "" {
		return nil, fmt.Errorf("github: empty token")
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	if apiURL == "" {
		apiURL = f.DefaultAPIURL
	}
	if apiURL == "" || apiURL == "https://api.github.com" {
		return gh.NewClient(tc), nil
	}
	// Enterprise: baseURL must end with /api/v3 for GitHub Enterprise Server.
	baseURL := strings.TrimRight(apiURL, "/")
	if !strings.HasSuffix(baseURL, "/api/v3") && !strings.HasSuffix(baseURL, "/api/v3/") {
		// only append for hosts that are clearly enterprise (not api.github.com).
		if !strings.Contains(baseURL, "api.github.com") {
			baseURL += "/api/v3"
		}
	}
	c, err := gh.NewClient(tc).WithEnterpriseURLs(baseURL, baseURL)
	if err != nil {
		return nil, fmt.Errorf("github: enterprise client: %w", err)
	}
	_ = http.StatusOK // keep net/http import for clarity
	return c, nil
}

// NewUnauthenticatedClient returns a client usable for unauthenticated
// requests (e.g. validating a token by calling /user).
func (f *ClientFactory) NewUnauthenticatedClient(apiURL string) *gh.Client {
	if apiURL == "" {
		apiURL = f.DefaultAPIURL
	}
	if apiURL == "" || apiURL == "https://api.github.com" {
		return gh.NewClient(http.DefaultClient)
	}
	c, _ := gh.NewClient(http.DefaultClient).WithEnterpriseURLs(apiURL, apiURL)
	return c
}

// EncryptedAccessor combines an encrypted account record with the encryption
// service, so callers can transparently obtain a plaintext token without
// leaking it into logs.
type EncryptedAccessor struct {
	EncryptedToken string
	APIURL         string
	Enc            *encryption.Service
}

// Decrypt returns the plaintext token. Callers must not log the result.
func (a *EncryptedAccessor) Decrypt() (string, error) {
	if a.Enc == nil {
		return "", fmt.Errorf("github: encryption service is nil")
	}
	return a.Enc.Decrypt(a.EncryptedToken)
}
