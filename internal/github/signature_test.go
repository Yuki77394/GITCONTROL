package github

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestVerifyWebhookSignatureSHA256 verifies that a correctly-signed request
// passes verification.
func TestVerifyWebhookSignatureSHA256(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	secret := "my_webhook_secret"
	sig := ComputeExpectedSignature256(body, []byte(secret))

	req := httptest.NewRequest(http.MethodPost, "/webhook/abc", nil)
	req.Body = http.NoBody
	req.Header.Set("X-Hub-Signature-256", sig)

	if err := VerifyWebhookSignature(req, body, secret); err != nil {
		t.Errorf("expected OK, got: %v", err)
	}
}

// TestVerifyWebhookSignatureTampered verifies that a tampered body fails.
func TestVerifyWebhookSignatureTampered(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	secret := "my_webhook_secret"
	sig := ComputeExpectedSignature256(body, []byte(secret))

	tampered := []byte(`{"action":"closed"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/abc", nil)
	req.Body = http.NoBody
	req.Header.Set("X-Hub-Signature-256", sig)

	if err := VerifyWebhookSignature(req, tampered, secret); err == nil {
		t.Errorf("expected signature verification to fail on tampered body")
	}
}

// TestVerifyWebhookSignatureWrongSecret verifies that a wrong secret fails.
func TestVerifyWebhookSignatureWrongSecret(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	sig := ComputeExpectedSignature256(body, []byte("correct_secret"))

	req := httptest.NewRequest(http.MethodPost, "/webhook/abc", nil)
	req.Body = http.NoBody
	req.Header.Set("X-Hub-Signature-256", sig)

	if err := VerifyWebhookSignature(req, body, "wrong_secret"); err == nil {
		t.Errorf("expected verification to fail with wrong secret")
	}
}

// TestVerifyWebhookSignatureMissingHeader verifies that missing signature
// headers are rejected.
func TestVerifyWebhookSignatureMissingHeader(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/abc", nil)
	req.Body = http.NoBody
	if err := VerifyWebhookSignature(req, body, "secret"); err == nil {
		t.Errorf("expected error for missing signature header")
	}
}

// TestVerifyWebhookSignatureEmptySecret verifies that an empty secret is
// rejected.
func TestVerifyWebhookSignatureEmptySecret(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/abc", nil)
	req.Body = http.NoBody
	req.Header.Set("X-Hub-Signature-256", "sha256=abc")
	if err := VerifyWebhookSignature(req, body, ""); err == nil {
		t.Errorf("expected error for empty secret")
	}
}

// TestGenerateStateUniqueness verifies that GenerateState returns unique
// values across multiple calls.
func TestGenerateStateUniqueness(t *testing.T) {
	const n = 100
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		s, err := GenerateState()
		if err != nil {
			t.Fatalf("GenerateState: %v", err)
		}
		if len(s) != 32 {
			t.Errorf("state length = %d, want 32", len(s))
		}
		if seen[s] {
			t.Errorf("duplicate state after %d calls: %s", i+1, s)
		}
		seen[s] = true
	}
}

// TestValidateRedirectURL ensures the OAuth callback URL is validated.
func TestValidateRedirectURL(t *testing.T) {
	valid := []string{
		"https://example.com/oauth/callback",
		"https://bot.swaggymusic.com/oauth/callback",
	}
	for _, u := range valid {
		if err := ValidateRedirectURL(u); err != nil {
			t.Errorf("ValidateRedirectURL(%q): unexpected err: %v", u, err)
		}
	}

	invalid := []string{
		"", "not-a-url", "http://example.com/oauth/callback", // http (not localhost)
		"https://example.com/wrong/path",
		"ftp://example.com/oauth/callback",
	}
	for _, u := range invalid {
		if err := ValidateRedirectURL(u); err == nil {
			t.Errorf("ValidateRedirectURL(%q): expected error", u)
		}
	}

	// Localhost http is allowed.
	if err := ValidateRedirectURL("http://localhost:8080/oauth/callback"); err != nil {
		t.Errorf("expected localhost http to be allowed: %v", err)
	}
}

// ComputeExpectedSignature256 returns the "sha256=<hex>" HMAC-SHA-256 of
// body using secret. Used by tests.
func ComputeExpectedSignature256(body, secret []byte) string {
	return ComputeExpectedSignature(body, secret)
}
