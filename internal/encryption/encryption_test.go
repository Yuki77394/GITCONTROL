package encryption

import (
	"strings"
	"testing"
)

// TestEncryptDecryptRoundTrip ensures that data encrypted by Encrypt can be
// decrypted by Decrypt with the same key, and that the round-trip is exact.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	svc, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cases := []string{
		"",
		"a",
		"hello world",
		"ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		strings.Repeat("x", 4096),
		"unicode: 你好 🌍",
	}

	for _, in := range cases {
		enc, err := svc.Encrypt(in)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", in, err)
		}
		if enc == in && in != "" {
			t.Errorf("Encrypt did not change input %q", in)
		}
		dec, err := svc.Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if dec != in {
			t.Errorf("round-trip mismatch: got %q want %q", dec, in)
		}
	}
}

// TestEncryptUniqueNonce verifies that encrypting the same plaintext twice
// produces different ciphertexts (because the nonce is random).
func TestEncryptUniqueNonce(t *testing.T) {
	key := make([]byte, 32)
	svc, _ := New(key)
	a, _ := svc.Encrypt("same plaintext")
	b, _ := svc.Encrypt("same plaintext")
	if a == b {
		t.Errorf("expected different ciphertexts due to random nonce")
	}
}

// TestDecryptWrongKey ensures that decryption with the wrong key fails.
func TestDecryptWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 1
	svc1, _ := New(key1)
	svc2, _ := New(key2)

	enc, _ := svc1.Encrypt("secret")
	if _, err := svc2.Decrypt(enc); err == nil {
		t.Errorf("expected decryption with wrong key to fail")
	}
}

// TestDecryptTamperedCiphertext ensures that tampered ciphertext fails
// authentication.
func TestDecryptTamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	svc, _ := New(key)
	enc, _ := svc.Encrypt("secret")

	// Tamper: flip a bit in the middle of the ciphertext.
	tampered := enc[:len(enc)-4] + "AAAA"
	if _, err := svc.Decrypt(tampered); err == nil {
		t.Errorf("expected tampered ciphertext to fail authentication")
	}
}

// TestDecryptMalformedInput verifies that invalid base64 / too-short
// ciphertexts are rejected.
func TestDecryptMalformedInput(t *testing.T) {
	key := make([]byte, 32)
	svc, _ := New(key)

	cases := []string{
		"not-base64!@#",
		"YWJjZA==", // valid base64 but too short
		"",
	}
	for _, in := range cases {
		if _, err := svc.Decrypt(in); err == nil {
			t.Errorf("expected error for malformed input %q", in)
		}
	}
}

// TestNewInvalidKey ensures that non-32-byte keys are rejected.
func TestNewInvalidKey(t *testing.T) {
	cases := [][]byte{
		nil,
		[]byte("short"),
		make([]byte, 16),
		make([]byte, 24),
		make([]byte, 64),
	}
	for _, k := range cases {
		if _, err := New(k); err == nil {
			t.Errorf("expected error for key length %d", len(k))
		}
	}
}
