package webhookroutes

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateRouteID(t *testing.T) {
	id, err := GenerateRouteID()
	if err != nil {
		t.Fatalf("GenerateRouteID: %v", err)
	}
	if len(id) != 64 {
		t.Errorf("route ID length = %d, want 64 (32 bytes hex)", len(id))
	}
	for _, c := range id {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("route ID contains non-hex char %q in %q", c, id)
			break
		}
	}
}

func TestGenerateRouteIDUnique(t *testing.T) {
	const n = 100
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		id, err := GenerateRouteID()
		if err != nil {
			t.Fatalf("GenerateRouteID: %v", err)
		}
		if seen[id] {
			t.Errorf("duplicate route ID after %d calls", i+1)
		}
		seen[id] = true
	}
}

// TestStoreLifecycle is a structural test that verifies the Store type and
// its methods have the expected signatures (compile-time API check).
// It does NOT require a live MongoDB.
func TestStoreLifecycle(t *testing.T) {
	var s *Store
	ctx := context.Background()
	// Reference the methods to ensure they exist with the right signatures.
	_ = s.Create
	_ = s.Lookup
	_ = s.Rotate
	_ = s.Revoke
	_ = s.Delete
	_ = ctx
}
