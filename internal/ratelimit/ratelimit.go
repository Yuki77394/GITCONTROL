// Package ratelimit provides a simple per-key token-bucket rate limiter
// used to throttle Telegram commands and GitHub API calls per user.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a per-key token bucket.
type Limiter struct {
	mu        sync.Mutex
	buckets   map[int64]*bucket
	maxTokens int
	refillPer time.Duration
}

type bucket struct {
	tokens   float64
	lastTime time.Time
}

// New creates a Limiter allowing maxTokens per minute (per key).
func New(maxTokensPerMin int) *Limiter {
	if maxTokensPerMin <= 0 {
		maxTokensPerMin = 20
	}
	return &Limiter{
		buckets:   make(map[int64]*bucket),
		maxTokens: maxTokensPerMin,
		refillPer: time.Minute,
	}
}

// Allow returns true if the key may proceed, consuming one token.
// Returns false (and does not consume) if the bucket is empty.
func (l *Limiter) Allow(key int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(l.maxTokens), lastTime: time.Now()}
		l.buckets[key] = b
	}
	now := time.Now()
	elapsed := now.Sub(b.lastTime)
	refill := float64(elapsed) / float64(l.refillPer) * float64(l.maxTokens)
	if refill > 0 {
		b.tokens = minFloat(float64(l.maxTokens), b.tokens+refill)
		b.lastTime = now
	}
	if b.tokens < 1.0 {
		return false
	}
	b.tokens -= 1.0
	return true
}

// Reset clears the bucket for a key.
func (l *Limiter) Reset(key int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
