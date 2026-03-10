package gitclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFileContentFetcher_GitHub(t *testing.T) {
	content := "name: test\nsteps:\n  - name: build\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/repos/owner/repo/contents/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("ref") != "abc123" {
			t.Errorf("unexpected ref: %s", r.URL.Query().Get("ref"))
		}
		// Verify auth header
		auth := r.Header.Get("Authorization")
		if !strings.Contains(auth, "ghp_test") {
			t.Errorf("expected token in auth header, got %q", auth)
		}

		json.NewEncoder(w).Encode(map[string]string{
			"content":  encoded,
			"encoding": "base64",
		})
	}))
	defer server.Close()

	// Override GitHub API URL by creating a fetcher that hits our test server
	fetcher := &FileContentFetcher{}
	result, err := fetcher.fetchAndDecode(context.Background(), "ghp_test",
		server.URL+"/repos/owner/repo/contents/.featherci%2Fworkflow.yml?ref=abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != content {
		t.Errorf("got %q, want %q", string(result), content)
	}
}

func TestFileContentFetcher_GitLab(t *testing.T) {
	content := "steps:\n  - name: test\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/v4/projects/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"content":  encoded,
			"encoding": "base64",
		})
	}))
	defer server.Close()

	fetcher := NewFileContentFetcher(server.URL, "")
	result, err := fetcher.GetFileContent(context.Background(), "gitlab", "glpat-test", "owner/repo", ".featherci/workflow.yml", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != content {
		t.Errorf("got %q, want %q", string(result), content)
	}
}

func TestFileContentFetcher_Gitea(t *testing.T) {
	content := "steps:\n  - name: deploy\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/v1/repos/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"content":  encoded,
			"encoding": "base64",
		})
	}))
	defer server.Close()

	fetcher := NewFileContentFetcher("", server.URL)
	result, err := fetcher.GetFileContent(context.Background(), "gitea", "tok-test", "owner/repo", ".featherci/workflow.yml", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != content {
		t.Errorf("got %q, want %q", string(result), content)
	}
}

func TestFileContentFetcher_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer server.Close()

	fetcher := &FileContentFetcher{}
	_, err := fetcher.fetchAndDecode(context.Background(), "tok", server.URL+"/notfound")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestFileContentFetcher_UnsupportedProvider(t *testing.T) {
	fetcher := NewFileContentFetcher("", "")
	_, err := fetcher.GetFileContent(context.Background(), "bitbucket", "tok", "owner/repo", "file.yml", "main")
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestFileContentFetcher_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	fetcher := &FileContentFetcher{}
	_, err := fetcher.fetchAndDecode(context.Background(), "tok", server.URL+"/error")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected status 500 in error, got: %v", err)
	}
}
