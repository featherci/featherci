package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// redirectTransport intercepts all HTTP requests and sends them to a test server.
type redirectTransport struct {
	server *httptest.Server
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.server.URL, "http://")
	return http.DefaultTransport.RoundTrip(req)
}

// withRedirectClient temporarily replaces http.DefaultClient to redirect all traffic to srv.
// Returns a cleanup function to restore the original client.
func withRedirectClient(srv *httptest.Server) func() {
	orig := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: &redirectTransport{server: srv}}
	return func() { http.DefaultClient = orig }
}

func testBuildEvent(status string) BuildEvent {
	return BuildEvent{
		ProjectName:   "testproj",
		BuildNumber:   42,
		Status:        status,
		Branch:        "main",
		CommitSHA:     "abcdef1234567890",
		CommitMessage: "test commit",
		CommitAuthor:  "dev",
		Duration:      30 * time.Second,
		BuildURL:      "https://ci.example.com/builds/42",
	}
}

// ---------------------------------------------------------------------------
// Slack (slack.go)
// ---------------------------------------------------------------------------

func TestNewSlackNotifier_MissingWebhookURL(t *testing.T) {
	_, err := NewSlackNotifier(map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing webhook_url")
	}
}

func TestNewSlackNotifier_Valid(t *testing.T) {
	n, err := NewSlackNotifier(map[string]string{"webhook_url": "https://hooks.slack.com/test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
}

func TestSlackNotifier_Send_Success(t *testing.T) {
	var received bool
	var contentType string
	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		contentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n, err := NewSlackNotifier(map[string]string{"webhook_url": srv.URL})
	if err != nil {
		t.Fatalf("NewSlackNotifier error: %v", err)
	}

	event := BuildEvent{
		ProjectName:   "myproj",
		BuildNumber:   5,
		Status:        "success",
		Branch:        "main",
		CommitSHA:     "abcdef1234567890",
		CommitMessage: "test commit",
		CommitAuthor:  "dev",
		Duration:      30 * time.Second,
		BuildURL:      "https://ci.example.com/builds/5",
	}

	err = n.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if !received {
		t.Error("server did not receive request")
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}
	if _, ok := body["attachments"]; !ok {
		t.Error("request body missing 'attachments' key")
	}
}

func TestSlackNotifier_Send_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n, _ := NewSlackNotifier(map[string]string{"webhook_url": srv.URL})
	err := n.Send(context.Background(), BuildEvent{Status: "failure"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// ---------------------------------------------------------------------------
// Discord (discord.go)
// ---------------------------------------------------------------------------

func TestNewDiscordNotifier_MissingWebhookURL(t *testing.T) {
	_, err := NewDiscordNotifier(map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing webhook_url")
	}
}

func TestNewDiscordNotifier_Valid(t *testing.T) {
	n, err := NewDiscordNotifier(map[string]string{"webhook_url": "https://discord.com/api/webhooks/test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
}

func TestDiscordNotifier_Send_Success(t *testing.T) {
	var received bool
	var contentType string
	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		contentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n, err := NewDiscordNotifier(map[string]string{"webhook_url": srv.URL})
	if err != nil {
		t.Fatalf("NewDiscordNotifier error: %v", err)
	}

	event := BuildEvent{
		ProjectName:   "myproj",
		BuildNumber:   10,
		Status:        "failure",
		Branch:        "develop",
		CommitSHA:     "deadbeef12345678",
		CommitMessage: "break things",
		CommitAuthor:  "dev",
		Duration:      1*time.Minute + 15*time.Second,
		BuildURL:      "https://ci.example.com/builds/10",
	}

	err = n.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if !received {
		t.Error("server did not receive request")
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}
	if _, ok := body["embeds"]; !ok {
		t.Error("request body missing 'embeds' key")
	}
}

func TestDiscordNotifier_Send_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n, _ := NewDiscordNotifier(map[string]string{"webhook_url": srv.URL})
	err := n.Send(context.Background(), BuildEvent{Status: "success"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// ---------------------------------------------------------------------------
// Pushover (pushover.go) — constructor validation only (hardcoded URL)
// ---------------------------------------------------------------------------

func TestNewPushoverNotifier_MissingAppToken(t *testing.T) {
	_, err := NewPushoverNotifier(map[string]string{"user_key": "key"})
	if err == nil {
		t.Fatal("expected error for missing app_token")
	}
}

func TestNewPushoverNotifier_MissingUserKey(t *testing.T) {
	_, err := NewPushoverNotifier(map[string]string{"app_token": "tok"})
	if err == nil {
		t.Fatal("expected error for missing user_key")
	}
}

func TestNewPushoverNotifier_Valid(t *testing.T) {
	n, err := NewPushoverNotifier(map[string]string{"app_token": "tok", "user_key": "key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
}

func TestPushoverNotifier_Send(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cleanup := withRedirectClient(srv)
	defer cleanup()

	n, _ := NewPushoverNotifier(map[string]string{"app_token": "my-token", "user_key": "my-user"})
	err := n.Send(context.Background(), testBuildEvent("success"))
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if !strings.Contains(receivedBody, "token=my-token") {
		t.Errorf("body missing token, got: %s", receivedBody)
	}
	if !strings.Contains(receivedBody, "user=my-user") {
		t.Errorf("body missing user, got: %s", receivedBody)
	}
	if !strings.Contains(receivedBody, "priority=0") {
		t.Errorf("body missing priority=0, got: %s", receivedBody)
	}
}

func TestPushoverNotifier_Send_FailurePriority(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cleanup := withRedirectClient(srv)
	defer cleanup()

	n, _ := NewPushoverNotifier(map[string]string{"app_token": "tok", "user_key": "key"})
	n.Send(context.Background(), testBuildEvent("failure"))
	if !strings.Contains(receivedBody, "priority=1") {
		t.Errorf("body missing priority=1 for failure, got: %s", receivedBody)
	}
}

func TestPushoverNotifier_Send_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	cleanup := withRedirectClient(srv)
	defer cleanup()

	n, _ := NewPushoverNotifier(map[string]string{"app_token": "tok", "user_key": "key"})
	err := n.Send(context.Background(), testBuildEvent("success"))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// ---------------------------------------------------------------------------
// SendGrid (sendgrid.go)
// ---------------------------------------------------------------------------

func TestNewSendgridNotifier_MissingAPIKey(t *testing.T) {
	_, err := NewSendgridNotifier(map[string]string{"from": "a@b.com", "to": "c@d.com"})
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
}

func TestNewSendgridNotifier_MissingFrom(t *testing.T) {
	_, err := NewSendgridNotifier(map[string]string{"api_key": "key", "to": "c@d.com"})
	if err == nil {
		t.Fatal("expected error for missing from")
	}
}

func TestNewSendgridNotifier_MissingTo(t *testing.T) {
	_, err := NewSendgridNotifier(map[string]string{"api_key": "key", "from": "a@b.com"})
	if err == nil {
		t.Fatal("expected error for missing to")
	}
}

func TestNewSendgridNotifier_Valid(t *testing.T) {
	n, err := NewSendgridNotifier(map[string]string{"api_key": "key", "from": "a@b.com", "to": "c@d.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
}

func TestNewSendgridNotifier_MultipleRecipients(t *testing.T) {
	n, err := NewSendgridNotifier(map[string]string{
		"api_key": "key", "from": "a@b.com", "to": "c@d.com, e@f.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(n.to) != 2 {
		t.Errorf("expected 2 recipients, got %d", len(n.to))
	}
	if n.to[1] != "e@f.com" {
		t.Errorf("second recipient = %q, want %q", n.to[1], "e@f.com")
	}
}

func TestSendgridNotifier_Send(t *testing.T) {
	var authHeader string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	cleanup := withRedirectClient(srv)
	defer cleanup()

	n, _ := NewSendgridNotifier(map[string]string{
		"api_key": "sg-test-key", "from": "ci@test.com", "to": "dev@test.com",
	})
	err := n.Send(context.Background(), testBuildEvent("success"))
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if authHeader != "Bearer sg-test-key" {
		t.Errorf("Authorization = %q, want %q", authHeader, "Bearer sg-test-key")
	}
	if body["subject"] == nil {
		t.Error("body missing subject")
	}
}

func TestSendgridNotifier_Send_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	cleanup := withRedirectClient(srv)
	defer cleanup()

	n, _ := NewSendgridNotifier(map[string]string{
		"api_key": "key", "from": "a@b.com", "to": "c@d.com",
	})
	err := n.Send(context.Background(), testBuildEvent("failure"))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// ---------------------------------------------------------------------------
// Mailgun (mailgun.go)
// ---------------------------------------------------------------------------

func TestNewMailgunNotifier_MissingAPIKey(t *testing.T) {
	_, err := NewMailgunNotifier(map[string]string{"domain": "mg.test.com", "from": "a@b.com", "to": "c@d.com"})
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
}

func TestNewMailgunNotifier_MissingDomain(t *testing.T) {
	_, err := NewMailgunNotifier(map[string]string{"api_key": "key", "from": "a@b.com", "to": "c@d.com"})
	if err == nil {
		t.Fatal("expected error for missing domain")
	}
}

func TestNewMailgunNotifier_MissingFrom(t *testing.T) {
	_, err := NewMailgunNotifier(map[string]string{"api_key": "key", "domain": "mg.test.com", "to": "c@d.com"})
	if err == nil {
		t.Fatal("expected error for missing from")
	}
}

func TestNewMailgunNotifier_MissingTo(t *testing.T) {
	_, err := NewMailgunNotifier(map[string]string{"api_key": "key", "domain": "mg.test.com", "from": "a@b.com"})
	if err == nil {
		t.Fatal("expected error for missing to")
	}
}

func TestNewMailgunNotifier_Valid(t *testing.T) {
	n, err := NewMailgunNotifier(map[string]string{
		"api_key": "key", "domain": "mg.test.com", "from": "a@b.com", "to": "c@d.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
}

func TestNewMailgunNotifier_MultipleRecipients(t *testing.T) {
	n, err := NewMailgunNotifier(map[string]string{
		"api_key": "key", "domain": "mg.test.com", "from": "a@b.com", "to": "c@d.com, e@f.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(n.to) != 2 {
		t.Errorf("expected 2 recipients, got %d", len(n.to))
	}
}

func TestMailgunNotifier_Send(t *testing.T) {
	var receivedBody string
	var authUser, authPass string
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authUser, authPass, _ = r.BasicAuth()
		contentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cleanup := withRedirectClient(srv)
	defer cleanup()

	n, _ := NewMailgunNotifier(map[string]string{
		"api_key": "mg-key-123", "domain": "mg.test.com", "from": "ci@test.com", "to": "dev@test.com",
	})
	err := n.Send(context.Background(), testBuildEvent("success"))
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if authUser != "api" || authPass != "mg-key-123" {
		t.Errorf("BasicAuth = (%q, %q), want (%q, %q)", authUser, authPass, "api", "mg-key-123")
	}
	if contentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want form-urlencoded", contentType)
	}
	if !strings.Contains(receivedBody, "from=") {
		t.Error("body missing from field")
	}
}

func TestMailgunNotifier_Send_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	cleanup := withRedirectClient(srv)
	defer cleanup()

	n, _ := NewMailgunNotifier(map[string]string{
		"api_key": "key", "domain": "mg.test.com", "from": "a@b.com", "to": "c@d.com",
	})
	err := n.Send(context.Background(), testBuildEvent("failure"))
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

// ---------------------------------------------------------------------------
// SMTP constructor validation (email_smtp.go) — no Send test
// ---------------------------------------------------------------------------

func TestNewSMTPNotifier_MissingHost(t *testing.T) {
	_, err := NewSMTPNotifier(map[string]string{"from": "a@b.com", "to": "c@d.com"})
	if err == nil {
		t.Fatal("expected error for missing host")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("error should mention host: %v", err)
	}
}

func TestNewSMTPNotifier_MissingFrom(t *testing.T) {
	_, err := NewSMTPNotifier(map[string]string{"host": "smtp.test.com", "to": "c@d.com"})
	if err == nil {
		t.Fatal("expected error for missing from")
	}
}

func TestNewSMTPNotifier_MissingTo(t *testing.T) {
	_, err := NewSMTPNotifier(map[string]string{"host": "smtp.test.com", "from": "a@b.com"})
	if err == nil {
		t.Fatal("expected error for missing to")
	}
}

func TestNewSMTPNotifier_DefaultPort(t *testing.T) {
	n, err := NewSMTPNotifier(map[string]string{
		"host": "smtp.test.com", "from": "a@b.com", "to": "c@d.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.port != "587" {
		t.Errorf("default port = %q, want %q", n.port, "587")
	}
}

func TestNewSMTPNotifier_Valid(t *testing.T) {
	n, err := NewSMTPNotifier(map[string]string{
		"host": "smtp.test.com", "port": "465", "from": "a@b.com", "to": "c@d.com",
		"username": "user", "password": "pass",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
	if n.port != "465" {
		t.Errorf("port = %q, want %q", n.port, "465")
	}
}
