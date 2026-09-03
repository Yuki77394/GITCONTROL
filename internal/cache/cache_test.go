package cache

import (
	"testing"
	"time"
)

func TestCacheSetGet(t *testing.T) {
	c := New[string, int]()
	c.Set("a", 1, 1*time.Minute)
	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Errorf("Get(a) = %d, %v; want 1, true", v, ok)
	}
}

func TestCacheMiss(t *testing.T) {
	c := New[string, int]()
	_, ok := c.Get("missing")
	if ok {
		t.Errorf("expected cache miss")
	}
}

func TestCacheExpiry(t *testing.T) {
	c := New[string, int]()
	c.Set("temp", 42, 50*time.Millisecond)
	v, ok := c.Get("temp")
	if !ok || v != 42 {
		t.Errorf("before expiry: got %d, %v; want 42, true", v, ok)
	}
	time.Sleep(100 * time.Millisecond)
	_, ok = c.Get("temp")
	if ok {
		t.Errorf("expected expiry after 100ms")
	}
}

func TestCacheDelete(t *testing.T) {
	c := New[string, int]()
	c.Set("x", 10, 1*time.Minute)
	c.Delete("x")
	if _, ok := c.Get("x"); ok {
		t.Errorf("expected item to be deleted")
	}
}

func TestCacheLen(t *testing.T) {
	c := New[int, string]()
	c.Set(1, "a", 1*time.Minute)
	c.Set(2, "b", 1*time.Minute)
	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2", c.Len())
	}
}

func TestCacheCleanup(t *testing.T) {
	c := New[string, int]()
	c.Set("a", 1, 50*time.Millisecond)
	c.Set("b", 2, 1*time.Minute)
	time.Sleep(100 * time.Millisecond)
	c.Cleanup()
	if c.Len() != 1 {
		t.Errorf("after cleanup: Len = %d, want 1", c.Len())
	}
	if _, ok := c.Get("b"); !ok {
		t.Errorf("expected b to still be present")
	}
}
