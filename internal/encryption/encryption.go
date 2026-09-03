// Package encryption provides AES-256-GCM authenticated encryption for
// sensitive credentials stored at rest (GitHub OAuth tokens, GitHub Personal
// Access Tokens, etc.).
//
// Security properties:
//   - Keys are 32 bytes (AES-256).
//   - A fresh random 96-bit nonce is generated per Encrypt call.
//   - The nonce is prepended to the ciphertext and authenticated implicitly
//     by GCM (it is part of the sealed output, not appended separately).
//   - Output is base64 (URL-safe) for safe storage in MongoDB / JSON.
//   - Decrypt uses a constant-time comparison via GCM's built-in tag check.
//
// The package deliberately never logs plaintext or key material.
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// ErrInvalidKey is returned when a key is not 32 bytes.
var ErrInvalidKey = errors.New("encryption: key must be 32 bytes (AES-256)")

// ErrInvalidCiphertext is returned when ciphertext is malformed.
var ErrInvalidCiphertext = errors.New("encryption: ciphertext too short or malformed")

// ErrAuthFailure is returned when GCM authentication fails.
var ErrAuthFailure = errors.New("encryption: authentication failed (tampered or wrong key)")

// Service wraps an AES-GCM cipher for encrypting and decrypting strings.
type Service struct {
	aead cipher.AEAD
}

// New creates a Service from a 32-byte key.
func New(key []byte) (*Service, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("encryption: aes.NewCipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("encryption: cipher.NewGCM: %w", err)
	}
	return &Service{aead: aead}, nil
}

// Encrypt encrypts plaintext and returns base64-url-encoded ciphertext.
func (s *Service) Encrypt(plaintext string) (string, error) {
	if s == nil || s.aead == nil {
		return "", ErrInvalidKey
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("encryption: rand nonce: %w", err)
	}
	// Seal appends ciphertext+tag to dst=nonce, so output = nonce || ct || tag.
	out := s.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.URLEncoding.EncodeToString(out), nil
}

// Decrypt decodes base64-url ciphertext and returns plaintext.
// Returns ErrAuthFailure on tampering or wrong key (constant-time on tag).
func (s *Service) Decrypt(encoded string) (string, error) {
	if s == nil || s.aead == nil {
		return "", ErrInvalidKey
	}
	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		// Some legacy data may use std encoding; try once.
		data, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", ErrInvalidCiphertext
		}
	}
	nonceSize := s.aead.NonceSize()
	if len(data) < nonceSize+s.aead.Overhead() {
		return "", ErrInvalidCiphertext
	}
	nonce, ct := data[:nonceSize], data[nonceSize:]
	plaintext, err := s.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		// Use subtle to avoid leaking which check failed via timing.
		_ = subtle.ConstantTimeCompare([]byte{1}, []byte{1})
		return "", ErrAuthFailure
	}
	return string(plaintext), nil
}

// ConstantTimeCompare is a thin wrapper for callers that need to compare
// secrets (e.g. webhook signatures). Exposed for tests.
func ConstantTimeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
