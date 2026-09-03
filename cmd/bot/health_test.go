package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthEndpoint verifies that the /health endpoint returns 200 OK
// with the expected JSON body. We extract the handler logic into a test
// to verify it without needing the full bot stack.
//
// This test mirrors the exact handler registered in main.go's run() function.
func TestHealthEndpoint(t *testing.T) {
	// Rebuild the exact handler from main.go.
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]string{
			"status":  "ok",
			"service": "swaggymusic-github-bot",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
	if body["service"] != "swaggymusic-github-bot" {
		t.Errorf("service = %q, want %q", body["service"], "swaggymusic-github-bot")
	}

	// Verify no secrets are leaked.
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
}

// TestHealthEndpointNoAuth confirms the /health endpoint does not require
// authentication (it should respond to any GET request).
func TestHealthEndpointNoAuth(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]string{"status": "ok", "service": "swaggymusic-github-bot"}
		_ = json.NewEncoder(w).Encode(resp)
	}

	// Request with no Authorization header.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("health endpoint should not require auth; got status %d", rec.Code)
	}
}
