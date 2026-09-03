package validation

import (
	"strings"
	"testing"
)

func TestValidateRepoName(t *testing.T) {
	cases := []struct {
		in     string
		wantOk bool
	}{
		{"swaggymusic/github-bot", true},
		{"owner/repo", true},
		{"a/b", true},
		{"a.b/c-d", true},
		{"a_b/c.d", true},
		{"", false},
		{"single", false},
		{"a/b/c", false},
		{"a/", false},
		{"/b", false},
		{"a b/c", false},
		{"a/b c", false},
		{"a/b/../c", false},
	}
	for _, c := range cases {
		owner, repo, err := ValidateRepoName(c.in)
		if c.wantOk {
			if err != nil {
				t.Errorf("ValidateRepoName(%q): unexpected err: %v", c.in, err)
				continue
			}
			if owner == "" || repo == "" {
				t.Errorf("ValidateRepoName(%q): empty owner or repo", c.in)
			}
		} else {
			if err == nil {
				t.Errorf("ValidateRepoName(%q): expected error, got owner=%q repo=%q", c.in, owner, repo)
			}
		}
	}
}

func TestValidateBranchName(t *testing.T) {
	valid := []string{
		"main", "master", "feature/foo", "fix-123", "release-1.0",
		"v1.2.3", "feature_long_name",
	}
	for _, b := range valid {
		if err := ValidateBranchName(b); err != nil {
			t.Errorf("ValidateBranchName(%q): unexpected err: %v", b, err)
		}
	}

	invalid := []string{
		"", "  ", "foo bar", "foo..bar", "/foo", "foo/", ".foo", "foo.",
		"foo\nbar", "HEAD", strings.Repeat("a", 300),
	}
	for _, b := range invalid {
		if err := ValidateBranchName(b); err == nil {
			t.Errorf("ValidateBranchName(%q): expected error", b)
		}
	}
}

func TestValidateFilePath(t *testing.T) {
	valid := []string{
		"", "README.md", "src/main.go", "docs/intro.md",
		"a/b/c/d/e.txt", "file with spaces.txt",
	}
	for _, p := range valid {
		if err := ValidateFilePath(p); err != nil {
			t.Errorf("ValidateFilePath(%q): unexpected err: %v", p, err)
		}
	}

	invalid := []string{
		"/absolute", "../escape", "a/../../etc/passwd", "a\\b",
		"a\x00b", strings.Repeat("a", 2000),
	}
	for _, p := range invalid {
		if err := ValidateFilePath(p); err == nil {
			t.Errorf("ValidateFilePath(%q): expected error", p)
		}
	}
}

func TestValidateNumber(t *testing.T) {
	valid := []struct {
		in   string
		want int
	}{
		{"1", 1}, {"42", 42}, {"12345", 12345},
	}
	for _, c := range valid {
		n, err := ValidateNumber(c.in)
		if err != nil {
			t.Errorf("ValidateNumber(%q): unexpected err: %v", c.in, err)
		}
		if n != c.want {
			t.Errorf("ValidateNumber(%q): got %d want %d", c.in, n, c.want)
		}
	}

	invalid := []string{"", "0", "-1", "abc", "1.5", "1e3"}
	for _, s := range invalid {
		if _, err := ValidateNumber(s); err == nil {
			t.Errorf("ValidateNumber(%q): expected error", s)
		}
	}
}

func TestValidateGitHubAPIURL(t *testing.T) {
	// Default api.github.com always allowed.
	u, err := ValidateGitHubAPIURL("https://api.github.com", nil)
	if err != nil {
		t.Fatalf("ValidateGitHubAPIURL(api.github.com): %v", err)
	}
	if u != "https://api.github.com" {
		t.Errorf("got %q want https://api.github.com", u)
	}

	// Trailing slash stripped.
	u, _ = ValidateGitHubAPIURL("https://api.github.com/", nil)
	if u != "https://api.github.com" {
		t.Errorf("trailing slash not stripped: %q", u)
	}

	// HTTP rejected (except localhost).
	if _, err := ValidateGitHubAPIURL("http://example.com/api/v3", nil); err == nil {
		t.Errorf("expected error for http:// (non-localhost)")
	}

	// Localhost http allowed.
	if _, err := ValidateGitHubAPIURL("http://localhost:8080", nil); err != nil {
		t.Errorf("expected localhost http to be allowed: %v", err)
	}

	// Enterprise allowed if in allowlist.
	if _, err := ValidateGitHubAPIURL("https://github.example.com/api/v3", []string{"github.example.com"}); err != nil {
		t.Errorf("expected enterprise URL in allowlist to be allowed: %v", err)
	}

	// Enterprise rejected if NOT in allowlist.
	if _, err := ValidateGitHubAPIURL("https://github.example.com/api/v3", nil); err == nil {
		t.Errorf("expected enterprise URL not in allowlist to be rejected")
	}

	// Subdomain of allowlisted host is allowed.
	if _, err := ValidateGitHubAPIURL("https://api.github.example.com", []string{"github.example.com"}); err != nil {
		t.Errorf("expected subdomain of allowlisted host to be allowed: %v", err)
	}
}

func TestParseLabelArgs(t *testing.T) {
	cases := []struct {
		in        string
		wantAdds  []string
		wantRems  []string
		wantError bool
	}{
		{"+bug", []string{"bug"}, nil, false},
		{"+bug -wip", []string{"bug"}, []string{"wip"}, false},
		{"bug fix", []string{"bug", "fix"}, nil, false},
		{"+bug +fix -remove", []string{"bug", "fix"}, []string{"remove"}, false},
		{"-", nil, nil, true},
		{"+", nil, nil, true},
	}
	for _, c := range cases {
		adds, rems, err := ParseLabelArgs(strings.Fields(c.in))
		if c.wantError {
			if err == nil {
				t.Errorf("ParseLabelArgs(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseLabelArgs(%q): unexpected err: %v", c.in, err)
			continue
		}
		if !sliceEq(adds, c.wantAdds) {
			t.Errorf("ParseLabelArgs(%q): adds = %v, want %v", c.in, adds, c.wantAdds)
		}
		if !sliceEq(rems, c.wantRems) {
			t.Errorf("ParseLabelArgs(%q): rems = %v, want %v", c.in, rems, c.wantRems)
		}
	}
}

func TestValidateRepoURL(t *testing.T) {
	cases := []struct {
		in      string
		owner   string
		repo    string
		wantErr bool
	}{
		{"https://github.com/swaggymusic/github-bot", "swaggymusic", "github-bot", false},
		{"https://github.com/swaggymusic/github-bot/", "swaggymusic", "github-bot", false},
		{"http://github.com/swaggymusic/github-bot", "", "", true},
		{"https://github.com/single", "", "", true},
		{"https://github.com/a/b/c", "a", "b", false},
		{"not-a-url", "", "", true},
		{"", "", "", true},
	}
	for _, c := range cases {
		owner, repo, err := ValidateRepoURL(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ValidateRepoURL(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ValidateRepoURL(%q): unexpected err: %v", c.in, err)
			continue
		}
		if owner != c.owner || repo != c.repo {
			t.Errorf("ValidateRepoURL(%q): got owner=%q repo=%q, want owner=%q repo=%q", c.in, owner, repo, c.owner, c.repo)
		}
	}
}

func TestValidateSHA(t *testing.T) {
	valid := []string{
		"e1d22f3c7e4a5b6c8d9e0f1a2b3c4d5e6f7a8b9c",
		"abc123def4567890abcdef1234567890abcdef12",
		"a1b2c3d4e5f60718293a4b5c6d7e8f9001020304",
	}
	for _, s := range valid {
		if err := ValidateSHA(s); err != nil {
			t.Errorf("ValidateSHA(%q): unexpected err: %v", s, err)
		}
	}
	invalid := []string{
		"", "short", "E1D22F3C7E4A5B6C8D9E0F1A2B3C4D5E6F7A8B9", // uppercase
		"g1d22f3c7e4a5b6c8d9e0f1a2b3c4d5e6f7a8b9c", // non-hex
	}
	for _, s := range invalid {
		if err := ValidateSHA(s); err == nil {
			t.Errorf("ValidateSHA(%q): expected error", s)
		}
	}
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
