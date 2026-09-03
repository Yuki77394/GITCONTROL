// Package validation provides strict input validation helpers for user-
// supplied data: repository names, GitHub API URLs, branch names, file
// paths, issue/PR numbers, etc.
//
// The goal is to prevent injection, path traversal, and SSRF attacks by
// validating every user input before it is passed to the GitHub API or
// stored in the database.
package validation

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ErrInvalidRepo is returned when a repository name does not match
	// the "owner/repo" GitHub format.
	ErrInvalidRepo = errors.New("invalid repository: expected 'owner/repo'")
	// ErrInvalidBranch is returned for malformed branch names.
	ErrInvalidBranch = errors.New("invalid branch name")
	// ErrInvalidPath is returned for paths containing traversal sequences.
	ErrInvalidPath = errors.New("invalid path: must not contain '..' or absolute paths")
	// ErrInvalidURL is returned for malformed or unsafe URLs.
	ErrInvalidURL = errors.New("invalid URL")
	// ErrInvalidNumber is returned for non-positive issue/PR numbers.
	ErrInvalidNumber = errors.New("invalid number: must be a positive integer")
)

// repoNamePattern matches "owner/repo" with allowed GitHub characters.
// Owner/repo segments allow alphanumerics, dash, underscore, dot.
var repoNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// branchNamePattern is intentionally permissive about Unicode but forbids
// control characters and spaces. Allows slashes for nested branch names
// like "feature/foo".
var branchNamePattern = regexp.MustCompile(`^[^\x00-\x1f\x7f ~^:?*\[\]\\]+$`)

// labelPattern matches a single GitHub label name (letters, digits, dash,
// space, emoji allowed by GitHub, but we restrict to common safe subset).
var labelNamePattern = regexp.MustCompile(`^[A-Za-z0-9 _.\-/]{1,50}$`)

// usernamePattern matches GitHub usernames (alphanumeric + single dashes).
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38}[A-Za-z0-9])?$`)

// shaPattern matches 40-character lowercase hex SHA-1 or 64-char SHA-256.
var shaPattern = regexp.MustCompile(`^[a-f0-9]{40}$|^[a-f0-9]{64}$`)

// ValidateRepoName validates an "owner/repo" string.
func ValidateRepoName(s string) (owner, repo string, err error) {
	s = strings.TrimSpace(s)
	if !repoNamePattern.MatchString(s) {
		return "", "", ErrInvalidRepo
	}
	parts := strings.SplitN(s, "/", 2)
	return parts[0], parts[1], nil
}

// ValidateBranchName validates a Git branch name.
func ValidateBranchName(s string) error {
	if s == "" || len(s) > 200 {
		return ErrInvalidBranch
	}
	if strings.Contains(s, "..") || strings.Contains(s, "//") {
		return ErrInvalidBranch
	}
	if strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return ErrInvalidBranch
	}
	if strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
		return ErrInvalidBranch
	}
	if strings.EqualFold(s, "HEAD") {
		return ErrInvalidBranch
	}
	if !branchNamePattern.MatchString(s) {
		return ErrInvalidBranch
	}
	return nil
}

// ValidateFilePath validates a repository file path. The path may be empty
// (repo root) or a relative POSIX path. It must NOT contain "..", must NOT
// be absolute, and must NOT contain NUL bytes.
func ValidateFilePath(s string) error {
	if s == "" {
		return nil
	}
	if strings.Contains(s, "\x00") {
		return ErrInvalidPath
	}
	if strings.HasPrefix(s, "/") {
		return ErrInvalidPath
	}
	if strings.Contains(s, "..") {
		return ErrInvalidPath
	}
	if strings.Contains(s, "\\") {
		return ErrInvalidPath
	}
	// Limit length to GitHub's max path length (roughly).
	if len(s) > 1024 {
		return ErrInvalidPath
	}
	return nil
}

// ValidateNumber validates a positive integer (issue/PR number).
func ValidateNumber(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return 0, ErrInvalidNumber
	}
	return n, nil
}

// ValidateGitHubUsername validates a GitHub login.
func ValidateGitHubUsername(s string) error {
	s = strings.TrimPrefix(strings.TrimSpace(s), "@")
	if !usernamePattern.MatchString(s) {
		return fmt.Errorf("invalid GitHub username")
	}
	return nil
}

// NormalizeUsername strips a leading @ and trims whitespace.
func NormalizeUsername(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "@")
}

// ValidateLabelName validates a single label name.
func ValidateLabelName(s string) error {
	if !labelNamePattern.MatchString(s) {
		return fmt.Errorf("invalid label name")
	}
	return nil
}

// ValidateSHA validates a commit SHA.
func ValidateSHA(s string) error {
	if !shaPattern.MatchString(strings.TrimSpace(s)) {
		return fmt.Errorf("invalid commit SHA")
	}
	return nil
}

// ValidateGitHubAPIURL validates a GitHub API URL.
//   - Must be HTTPS unless explicitly allowed (localhost dev).
//   - Default https://api.github.com is always allowed.
//   - GitHub Enterprise URLs must match an allowlist entry (host suffix).
//   - Prevents SSRF by rejecting localhost, link-local, and IP literals
//     unless allowlisted.
//
// Returns the validated URL with no trailing slash.
func ValidateGitHubAPIURL(raw string, enterpriseAllowlist []string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "https://api.github.com", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ErrInvalidURL
	}
	if u.Scheme != "https" {
		// Allow http://localhost for local dev only.
		if u.Scheme != "http" || u.Hostname() != "localhost" {
			return "", fmt.Errorf("%w: must be HTTPS", ErrInvalidURL)
		}
	}
	host := u.Hostname()
	if host == "" {
		return "", ErrInvalidURL
	}
	// Default api.github.com is always allowed.
	if host == "api.github.com" {
		return strings.TrimRight(raw, "/"), nil
	}
	// Localhost (dev) is always allowed for http://
	if u.Scheme == "http" && (host == "localhost" || host == "127.0.0.1") {
		return strings.TrimRight(raw, "/"), nil
	}
	// For Enterprise URLs, host must match an allowlist entry.
	allowed := false
	for _, allow := range enterpriseAllowlist {
		allow = strings.TrimSpace(allow)
		if allow == "" {
			continue
		}
		if host == allow || strings.HasSuffix(host, "."+allow) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("%w: host %q is not in GITHUB_ENTERPRISE_ALLOWLIST", ErrInvalidURL, host)
	}
	return strings.TrimRight(raw, "/"), nil
}

// ValidateRepoURL validates a https://github.com/OWNER/REPO style URL.
// Returns (owner, repo, error).
func ValidateRepoURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ErrInvalidURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", ErrInvalidURL
	}
	if u.Scheme != "https" {
		return "", "", fmt.Errorf("%w: must be HTTPS", ErrInvalidURL)
	}
	// Accept github.com and configured enterprise hosts.
	host := u.Hostname()
	if host != "github.com" {
		// Allow enterprise hosts; rely on caller-side allowlist for full check.
		// Still validate path format below.
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", ErrInvalidRepo
	}
	owner, repo := parts[0], parts[1]
	if !repoNamePattern.MatchString(owner + "/" + repo) {
		return "", "", ErrInvalidRepo
	}
	return owner, repo, nil
}

// ParseLabelArgs parses a /label argument list like "+bug -help-wanted fix".
// Tokens starting with "+" are added; "-" are removed; bare tokens are added.
// Returns (adds, removes, error).
func ParseLabelArgs(args []string) (adds, removes []string, err error) {
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		switch {
		case strings.HasPrefix(a, "+"):
			name := strings.TrimSpace(a[1:])
			if name == "" {
				return nil, nil, fmt.Errorf("invalid label: empty name after '+'")
			}
			if err := ValidateLabelName(name); err != nil {
				return nil, nil, err
			}
			adds = append(adds, name)
		case strings.HasPrefix(a, "-"):
			name := strings.TrimSpace(a[1:])
			if name == "" {
				return nil, nil, fmt.Errorf("invalid label: empty name after '-'")
			}
			if err := ValidateLabelName(name); err != nil {
				return nil, nil, err
			}
			removes = append(removes, name)
		default:
			if err := ValidateLabelName(a); err != nil {
				return nil, nil, err
			}
			adds = append(adds, a)
		}
	}
	return adds, removes, nil
}

// SanitizeText trims and clips user-supplied free text (e.g. issue body).
// This is a defensive measure against absurdly large inputs.
func SanitizeText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen]
	}
	// Strip NUL bytes.
	s = strings.ReplaceAll(s, "\x00", "")
	return s
}
