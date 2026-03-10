package git

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockCommandRunner records commands for verification.
type mockCommandRunner struct {
	calls   [][]string
	results []mockResult
	callIdx int
}

type mockResult struct {
	output []byte
	err    error
}

func (m *mockCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, append([]string{name}, args...))
	if m.callIdx < len(m.results) {
		r := m.results[m.callIdx]
		m.callIdx++
		return r.output, r.err
	}
	m.callIdx++
	return nil, nil
}

func TestCLIGitService_Clone(t *testing.T) {
	runner := &mockCommandRunner{}
	svc := NewCLIGitServiceWithRunner(runner)

	err := svc.Clone(context.Background(), "https://github.com/owner/repo.git", "tok123", "github", "/tmp/dest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}

	args := runner.calls[0]
	if args[0] != "git" || args[1] != "clone" || args[2] != "--depth=1" {
		t.Errorf("unexpected args: %v", args)
	}
	if !strings.Contains(args[3], "x-access-token:tok123@") {
		t.Errorf("expected token in URL, got %q", args[3])
	}
	if args[4] != "/tmp/dest" {
		t.Errorf("expected dest /tmp/dest, got %q", args[4])
	}
}

func TestCLIGitService_CloneTokenSanitized(t *testing.T) {
	runner := &mockCommandRunner{
		results: []mockResult{
			{err: fmt.Errorf("fatal: authentication failed for https://x-access-token:secret123@github.com/repo")},
		},
	}
	svc := NewCLIGitServiceWithRunner(runner)

	err := svc.Clone(context.Background(), "https://github.com/owner/repo.git", "secret123", "github", "/tmp/dest")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret123") {
		t.Errorf("error message contains token: %v", err)
	}
	if !strings.Contains(err.Error(), "***") {
		t.Errorf("expected sanitized token (***) in error: %v", err)
	}
}

func TestCLIGitService_Checkout_Direct(t *testing.T) {
	runner := &mockCommandRunner{}
	svc := NewCLIGitServiceWithRunner(runner)

	err := svc.Checkout(context.Background(), "/repo", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}

	args := runner.calls[0]
	expected := []string{"git", "-C", "/repo", "checkout", "abc123"}
	for i, v := range expected {
		if args[i] != v {
			t.Errorf("arg[%d]: got %q, want %q", i, args[i], v)
		}
	}
}

func TestCLIGitService_Checkout_ShallowFetch(t *testing.T) {
	runner := &mockCommandRunner{
		results: []mockResult{
			{err: fmt.Errorf("error: pathspec 'abc123' did not match")}, // direct checkout fails
			{}, // fetch succeeds
			{}, // checkout FETCH_HEAD succeeds
		},
	}
	svc := NewCLIGitServiceWithRunner(runner)

	err := svc.Checkout(context.Background(), "/repo", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(runner.calls))
	}

	// Verify fetch call
	fetchArgs := runner.calls[1]
	expected := []string{"git", "-C", "/repo", "fetch", "--depth=1", "origin", "abc123"}
	for i, v := range expected {
		if fetchArgs[i] != v {
			t.Errorf("fetch arg[%d]: got %q, want %q", i, fetchArgs[i], v)
		}
	}

	// Verify checkout FETCH_HEAD
	coArgs := runner.calls[2]
	if coArgs[4] != "FETCH_HEAD" {
		t.Errorf("expected checkout FETCH_HEAD, got %q", coArgs[4])
	}
}

func TestSanitizeTokenFromError(t *testing.T) {
	err := sanitizeTokenFromError(fmt.Errorf("failed with token mytoken123 in url"), "mytoken123")
	if strings.Contains(err.Error(), "mytoken123") {
		t.Errorf("token not sanitized: %v", err)
	}

	// nil error returns nil
	if sanitizeTokenFromError(nil, "tok") != nil {
		t.Error("expected nil for nil error")
	}

	// empty token returns error unchanged
	original := fmt.Errorf("some error")
	if sanitizeTokenFromError(original, "").Error() != "some error" {
		t.Error("expected unchanged error for empty token")
	}
}
