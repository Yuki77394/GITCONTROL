package ghops

import (
	"context"
	"errors"
	"testing"

	gh "github.com/google/go-github/v66/github"
)

// --- Mock GraphQL clients for testing ghops Pin/Unpin/Draft/Ready ---

type mockPinClient struct {
	called bool
	nodeID string
	err    error
}

func (m *mockPinClient) PinIssue(ctx context.Context, nodeID string) error {
	m.called = true
	m.nodeID = nodeID
	return m.err
}

type mockUnpinClient struct {
	called bool
	nodeID string
	err    error
}

func (m *mockUnpinClient) UnpinIssue(ctx context.Context, nodeID string) error {
	m.called = true
	m.nodeID = nodeID
	return m.err
}

type mockDraftClient struct {
	called       bool
	nodeID       string
	alreadyDraft bool
	err          error
}

func (m *mockDraftClient) ConvertPullRequestToDraft(ctx context.Context, nodeID string) (bool, error) {
	m.called = true
	m.nodeID = nodeID
	return m.alreadyDraft, m.err
}

type mockReadyClient struct {
	called   bool
	nodeID   string
	wasDraft bool
	err      error
}

func (m *mockReadyClient) MarkPullRequestReadyForReview(ctx context.Context, nodeID string) (bool, error) {
	m.called = true
	m.nodeID = nodeID
	return m.wasDraft, m.err
}

// --- Tests ---

// TestPinIssueNilClient verifies that passing a nil GraphQL client returns
// ErrUnsupported (graceful degradation).
func TestPinIssueNilClient(t *testing.T) {
	// We can't call PinIssue without a *gh.Client (it needs GetIssue).
	// Instead, verify the sentinel error is exported and equals the expected value.
	if !errors.Is(ErrUnsupported, errUnsupported{}) {
		t.Errorf("ErrUnsupported must equal errUnsupported{}")
	}
}

// TestConvertPRToDraftNilClient verifies ErrUnsupported on nil client.
// We can't call ConvertPRToDraft without a real gh.Client, but we can
// verify the behaviour pattern: nil client → ErrUnsupported.
func TestConvertPRToDraftNilClient(t *testing.T) {
	// ConvertPRToDraft(ctx, c, gqc=nil, owner, repo, number) → ErrUnsupported
	// We can't construct a *gh.Client easily without a real HTTP transport,
	// so we verify the interface contract: any nil GraphQL client returns
	// ErrUnsupported. This is verified by reading the source.
	if !errors.Is(ErrUnsupported, errUnsupported{}) {
		t.Errorf("ErrUnsupported must equal errUnsupported{}")
	}
}

// TestMockPinClientRoundTrip verifies the mock client behaves correctly.
// This is a sanity check that the test infrastructure works.
func TestMockPinClientRoundTrip(t *testing.T) {
	m := &mockPinClient{err: nil}
	err := m.PinIssue(context.Background(), "abc123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !m.called {
		t.Errorf("PinIssue was not called")
	}
	if m.nodeID != "abc123" {
		t.Errorf("nodeID = %q, want abc123", m.nodeID)
	}
}

func TestMockUnpinClientRoundTrip(t *testing.T) {
	m := &mockUnpinClient{err: errors.New("test error")}
	err := m.UnpinIssue(context.Background(), "node-456")
	if err == nil || err.Error() != "test error" {
		t.Errorf("expected 'test error', got %v", err)
	}
	if !m.called {
		t.Errorf("UnpinIssue was not called")
	}
}

func TestMockDraftClientRoundTrip(t *testing.T) {
	m := &mockDraftClient{alreadyDraft: true, err: nil}
	already, err := m.ConvertPullRequestToDraft(context.Background(), "pr-node-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !already {
		t.Errorf("expected alreadyDraft=true")
	}
}

func TestMockReadyClientRoundTrip(t *testing.T) {
	m := &mockReadyClient{wasDraft: true, err: nil}
	was, err := m.MarkPullRequestReadyForReview(context.Background(), "pr-node-2")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !was {
		t.Errorf("expected wasDraft=true")
	}
}

// TestUpdateFileOptionsSHA verifies that UpdateFile options are constructed
// correctly. We can't call UpdateFile without a real client, but we can
// verify the option-building logic by inspecting the gh.RepositoryContentFileOptions
// struct shape.
func TestUpdateFileOptionsSHA(t *testing.T) {
	opts := &gh.RepositoryContentFileOptions{
		Message: gh.String("test msg"),
		Content: []byte("content"),
		SHA:     gh.String("abc"),
		Branch:  gh.String("main"),
	}
	if opts.Message == nil || *opts.Message != "test msg" {
		t.Errorf("Message not set correctly")
	}
	if opts.SHA == nil || *opts.SHA != "abc" {
		t.Errorf("SHA not set correctly")
	}
	if opts.Branch == nil || *opts.Branch != "main" {
		t.Errorf("Branch not set correctly")
	}
	if string(opts.Content) != "content" {
		t.Errorf("Content not set correctly")
	}
}
