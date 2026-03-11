package notify

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// BuildEvent helpers (event.go)
// ---------------------------------------------------------------------------

func TestShortSHA(t *testing.T) {
	tests := []struct {
		name string
		sha  string
		want string
	}{
		{"normal long SHA", "abcdef1234567890", "abcdef12"},
		{"exactly 8 chars", "abcdef12", "abcdef12"},
		{"short SHA", "abc", "abc"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := BuildEvent{CommitSHA: tt.sha}
			if got := e.ShortSHA(); got != tt.want {
				t.Errorf("ShortSHA() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDurationString(t *testing.T) {
	tests := []struct {
		name string
		dur  time.Duration
		want string
	}{
		{"sub-second", 500 * time.Millisecond, "< 1s"},
		{"zero", 0, "< 1s"},
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 2*time.Minute + 30*time.Second, "2m30s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := BuildEvent{Duration: tt.dur}
			if got := e.DurationString(); got != tt.want {
				t.Errorf("DurationString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsSuccess(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"success", true},
		{"failure", false},
		{"cancelled", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			e := BuildEvent{Status: tt.status}
			if got := e.IsSuccess(); got != tt.want {
				t.Errorf("IsSuccess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCancelled(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"cancelled", true},
		{"success", false},
		{"failure", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			e := BuildEvent{Status: tt.status}
			if got := e.IsCancelled(); got != tt.want {
				t.Errorf("IsCancelled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"success", "Passed"},
		{"failure", "Failed"},
		{"cancelled", "Cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			e := BuildEvent{Status: tt.status}
			if got := e.StatusLabel(); got != tt.want {
				t.Errorf("StatusLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusEmoji(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"success", "\u2705"},
		{"failure", "\u274C"},
		{"cancelled", "\U0001F6AB"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			e := BuildEvent{Status: tt.status}
			if got := e.StatusEmoji(); got != tt.want {
				t.Errorf("StatusEmoji() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmailSubject(t *testing.T) {
	e := BuildEvent{
		ProjectName: "myproject",
		BuildNumber: 42,
		Status:      "success",
	}
	subj := e.EmailSubject()
	if !strings.Contains(subj, e.StatusEmoji()) {
		t.Errorf("EmailSubject() missing emoji: %q", subj)
	}
	if !strings.Contains(subj, "myproject") {
		t.Errorf("EmailSubject() missing project name: %q", subj)
	}
	if !strings.Contains(subj, "#42") {
		t.Errorf("EmailSubject() missing build number: %q", subj)
	}
	if !strings.Contains(subj, "Passed") {
		t.Errorf("EmailSubject() missing status label: %q", subj)
	}
}

// ---------------------------------------------------------------------------
// NewNotifier factory (notifier.go)
// ---------------------------------------------------------------------------

func TestNewNotifier_ValidTypes(t *testing.T) {
	configs := map[string]map[string]string{
		"email_smtp":     {"host": "smtp.example.com", "from": "a@b.com", "to": "c@d.com"},
		"email_sendgrid": {"api_key": "key", "from": "a@b.com", "to": "c@d.com"},
		"email_mailgun":  {"api_key": "key", "domain": "mg.example.com", "from": "a@b.com", "to": "c@d.com"},
		"slack":          {"webhook_url": "https://hooks.slack.com/test"},
		"discord":        {"webhook_url": "https://discord.com/api/webhooks/test"},
		"pushover":       {"app_token": "tok", "user_key": "key"},
	}
	for chType, cfg := range configs {
		t.Run(chType, func(t *testing.T) {
			n, err := NewNotifier(chType, cfg)
			if err != nil {
				t.Fatalf("NewNotifier(%q) error: %v", chType, err)
			}
			if n == nil {
				t.Fatalf("NewNotifier(%q) returned nil", chType)
			}
		})
	}
}

func TestNewNotifier_InvalidType(t *testing.T) {
	_, err := NewNotifier("carrier_pigeon", map[string]string{})
	if err == nil {
		t.Fatal("expected error for unknown channel type")
	}
}

func TestNewNotifier_MissingConfig(t *testing.T) {
	tests := []struct {
		chType string
		config map[string]string
	}{
		{"slack", map[string]string{}},
		{"discord", map[string]string{}},
		{"pushover", map[string]string{}},
		{"pushover", map[string]string{"app_token": "tok"}}, // missing user_key
		{"email_smtp", map[string]string{}},
		{"email_sendgrid", map[string]string{}},
		{"email_mailgun", map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.chType, func(t *testing.T) {
			_, err := NewNotifier(tt.chType, tt.config)
			if err == nil {
				t.Errorf("expected error for %q with config %v", tt.chType, tt.config)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ChannelTypes / ChannelTypeLabel (notifier.go)
// ---------------------------------------------------------------------------

func TestChannelTypes(t *testing.T) {
	types := ChannelTypes()
	if len(types) != 6 {
		t.Fatalf("ChannelTypes() returned %d items, want 6", len(types))
	}
	expected := map[string]bool{
		"email_smtp": true, "email_sendgrid": true, "email_mailgun": true,
		"slack": true, "discord": true, "pushover": true,
	}
	for _, ct := range types {
		if !expected[ct] {
			t.Errorf("unexpected channel type: %q", ct)
		}
	}
}

func TestChannelTypeLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"email_smtp", "Email (SMTP)"},
		{"email_sendgrid", "Email (SendGrid)"},
		{"email_mailgun", "Email (Mailgun)"},
		{"slack", "Slack"},
		{"discord", "Discord"},
		{"pushover", "Pushover"},
		{"unknown_thing", "unknown_thing"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ChannelTypeLabel(tt.input); got != tt.want {
				t.Errorf("ChannelTypeLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PreviewStore (preview.go)
// ---------------------------------------------------------------------------

func testEvent(status string) BuildEvent {
	return BuildEvent{
		ProjectName:   "testproj",
		BuildNumber:   1,
		Status:        status,
		Branch:        "main",
		CommitSHA:     "abcdef1234567890",
		CommitMessage: "fix things",
		CommitAuthor:  "dev",
		Duration:      10 * time.Second,
		BuildURL:      "https://ci.example.com/builds/1",
	}
}

func TestPreviewStore_CaptureAndList(t *testing.T) {
	s := NewPreviewStore()
	s.Capture("ch1", "slack", testEvent("success"), "", "")
	s.Capture("ch2", "discord", testEvent("failure"), "", "")

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("List() returned %d entries, want 2", len(list))
	}
	// newest first
	if list[0].ChannelName != "ch2" {
		t.Errorf("List()[0].ChannelName = %q, want %q", list[0].ChannelName, "ch2")
	}
	if list[1].ChannelName != "ch1" {
		t.Errorf("List()[1].ChannelName = %q, want %q", list[1].ChannelName, "ch1")
	}
}

func TestPreviewStore_Get(t *testing.T) {
	s := NewPreviewStore()
	s.Capture("ch1", "slack", testEvent("success"), "", "")

	entry, ok := s.Get(1)
	if !ok {
		t.Fatal("Get(1) returned false")
	}
	if entry.ChannelName != "ch1" {
		t.Errorf("Get(1).ChannelName = %q, want %q", entry.ChannelName, "ch1")
	}
}

func TestPreviewStore_GetUnknown(t *testing.T) {
	s := NewPreviewStore()
	_, ok := s.Get(999)
	if ok {
		t.Error("Get(999) returned true for non-existent entry")
	}
}

func TestPreviewStore_Clear(t *testing.T) {
	s := NewPreviewStore()
	s.Capture("ch1", "slack", testEvent("success"), "", "")
	s.Clear()
	list := s.List()
	if len(list) != 0 {
		t.Fatalf("List() after Clear() returned %d entries, want 0", len(list))
	}
}

func TestPreviewStore_Cap50(t *testing.T) {
	s := NewPreviewStore()
	for i := 0; i < 55; i++ {
		s.Capture("ch", "slack", testEvent("success"), "", "")
	}
	list := s.List()
	if len(list) != 50 {
		t.Fatalf("List() returned %d entries after 55 captures, want 50", len(list))
	}
}

func TestPreviewStore_CaptureEmail(t *testing.T) {
	s := NewPreviewStore()
	s.Capture("email-ch", "email_smtp", testEvent("success"), "from@test.com", "to@test.com")

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("List() returned %d entries, want 1", len(list))
	}
	entry := list[0]
	if entry.HTML == "" {
		t.Error("email capture should have rendered HTML")
	}
	if entry.Subject == "" {
		t.Error("email capture should have a subject")
	}
	if entry.From != "from@test.com" {
		t.Errorf("From = %q, want %q", entry.From, "from@test.com")
	}
	if entry.To != "to@test.com" {
		t.Errorf("To = %q, want %q", entry.To, "to@test.com")
	}
}

func TestPreviewStore_CaptureEmailTypes(t *testing.T) {
	for _, ct := range []string{"email_smtp", "email_sendgrid", "email_mailgun"} {
		s := NewPreviewStore()
		s.Capture("ch", ct, testEvent("failure"), "a@b.com", "c@d.com")
		entry, ok := s.Get(1)
		if !ok {
			t.Fatalf("Get(1) returned false for %s", ct)
		}
		if entry.HTML == "" {
			t.Errorf("%s capture should render HTML", ct)
		}
		if entry.Subject == "" {
			t.Errorf("%s capture should set Subject", ct)
		}
	}
}

func TestPreviewNotifier_Send(t *testing.T) {
	s := NewPreviewStore()
	pn := &previewNotifier{
		store:       s,
		channelName: "test-ch",
		channelType: "slack",
	}
	err := pn.Send(context.Background(), testEvent("success"))
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 captured entry, got %d", len(list))
	}
	if list[0].ChannelName != "test-ch" {
		t.Errorf("captured entry ChannelName = %q, want %q", list[0].ChannelName, "test-ch")
	}
}

// ---------------------------------------------------------------------------
// renderEmailHTML (email_template.go)
// ---------------------------------------------------------------------------

func TestRenderEmailHTML_Success(t *testing.T) {
	html, err := renderEmailHTML(testEvent("success"))
	if err != nil {
		t.Fatalf("renderEmailHTML() error: %v", err)
	}
	if !strings.Contains(html, "Build Passed") {
		t.Error("success email missing 'Build Passed'")
	}
	if !strings.Contains(html, "#16a34a") {
		t.Error("success email missing green color #16a34a")
	}
}

func TestRenderEmailHTML_Failure(t *testing.T) {
	html, err := renderEmailHTML(testEvent("failure"))
	if err != nil {
		t.Fatalf("renderEmailHTML() error: %v", err)
	}
	if !strings.Contains(html, "Build Failed") {
		t.Error("failure email missing 'Build Failed'")
	}
	if !strings.Contains(html, "#dc2626") {
		t.Error("failure email missing red color #dc2626")
	}
}

func TestRenderEmailHTML_Cancelled(t *testing.T) {
	html, err := renderEmailHTML(testEvent("cancelled"))
	if err != nil {
		t.Fatalf("renderEmailHTML() error: %v", err)
	}
	if !strings.Contains(html, "Build Cancelled") {
		t.Error("cancelled email missing 'Build Cancelled'")
	}
}
