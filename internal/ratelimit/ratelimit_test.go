package ratelimit

import (
	"testing"
)

func TestLimiterAllow(t *testing.T) {
	l := New(5)
	for i := 0; i < 5; i++ {
		if !l.Allow(1) {
			t.Errorf("call %d: expected Allow=true", i+1)
		}
	}
	if l.Allow(1) {
		t.Errorf("call 6: expected Allow=false (bucket empty)")
	}
}

func TestLimiterDifferentKeys(t *testing.T) {
	l := New(2)
	if !l.Allow(1) {
		t.Errorf("user 1 call 1: expected Allow=true")
	}
	if !l.Allow(2) {
		t.Errorf("user 2 call 1: expected Allow=true")
	}
	if !l.Allow(1) {
		t.Errorf("user 1 call 2: expected Allow=true")
	}
	if l.Allow(1) {
		t.Errorf("user 1 call 3: expected Allow=false")
	}
	if !l.Allow(2) {
		t.Errorf("user 2 call 2: expected Allow=true")
	}
}

func TestLimiterReset(t *testing.T) {
	l := New(1)
	l.Allow(1)
	if l.Allow(1) {
		t.Errorf("expected false after exhausting bucket")
	}
	l.Reset(1)
	if !l.Allow(1) {
		t.Errorf("expected true after reset")
	}
}
