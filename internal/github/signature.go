package github

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // GitHub webhook signatures use HMAC-SHA1.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// VerifyWebhookSignature verifies the X-Hub-Signature-256 (preferred) or
// X-Hub-Signature (SHA1, legacy) header against the request body.
//
// Uses constant-time comparison. Returns an error if verification fails or
// if neither header is present.
//
// The caller should read the body and pass it as `body` (the original body
// bytes, NOT a re-read of r.Body). The function does NOT consume r.Body.
func VerifyWebhookSignature(r *http.Request, body []byte, secret string) error {
	if secret == "" {
		return errors.New("webhook: secret is empty")
	}
	if body == nil {
		return errors.New("webhook: body is nil")
	}

	// Prefer SHA-256.
	if sig256 := r.Header.Get("X-Hub-Signature-256"); sig256 != "" {
		return verifyHMACSHA256(body, []byte(secret), sig256)
	}
	// Fall back to SHA-1 for older webhook configurations.
	if sig1 := r.Header.Get("X-Hub-Signature"); sig1 != "" {
		return verifyHMACSHA1(body, []byte(secret), sig1)
	}
	return errors.New("webhook: no signature header present")
}

func verifyHMACSHA256(body, secret []byte, sigHeader string) error {
	const prefix = "sha256="
	if !strings.HasPrefix(sigHeader, prefix) {
		return errors.New("webhook: invalid sha256 signature prefix")
	}
	got, err := hex.DecodeString(strings.TrimPrefix(sigHeader, prefix))
	if err != nil {
		return fmt.Errorf("webhook: invalid sha256 hex: %w", err)
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	want := mac.Sum(nil)
	if !hmac.Equal(got, want) {
		return errors.New("webhook: sha256 signature mismatch")
	}
	return nil
}

func verifyHMACSHA1(body, secret []byte, sigHeader string) error {
	const prefix = "sha1="
	if !strings.HasPrefix(sigHeader, prefix) {
		return errors.New("webhook: invalid sha1 signature prefix")
	}
	got, err := hex.DecodeString(strings.TrimPrefix(sigHeader, prefix))
	if err != nil {
		return fmt.Errorf("webhook: invalid sha1 hex: %w", err)
	}
	mac := hmac.New(sha1.New, secret) //nolint:gosec
	mac.Write(body)
	want := mac.Sum(nil)
	if !hmac.Equal(got, want) {
		return errors.New("webhook: sha1 signature mismatch")
	}
	return nil
}

// ReadBody reads the full request body and restores it so handlers later
// in the chain can re-read it.
func ReadBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	_ = r.Body.Close()
	return b, nil
}

// ParseHookID extracts the X-GitHub-Hook-ID header as int64.
func ParseHookID(r *http.Request) int64 {
	s := r.Header.Get("X-GitHub-Hook-ID")
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// EventType returns the X-GitHub-Event header value.
func EventType(r *http.Request) string {
	return r.Header.Get("X-GitHub-Event")
}

// DeliveryID returns the X-GitHub-Delivery header value.
func DeliveryID(r *http.Request) string {
	return r.Header.Get("X-GitHub-Delivery")
}

// ComputeExpectedSignature computes the expected HMAC-SHA-256 signature for
// the given body and secret, returned as the "sha256=<hex>" string. Used by
// tests and by the webhook server.
func ComputeExpectedSignature(body, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	sum := mac.Sum(nil)
	return "sha256=" + hex.EncodeToString(sum)
}
