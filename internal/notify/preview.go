package notify

import (
	"context"
	"sync"
	"time"
)

// PreviewEntry is a captured notification for dev-mode preview.
type PreviewEntry struct {
	ID          int
	ChannelName string
	ChannelType string
	Event       BuildEvent
	HTML        string // rendered email HTML (for email types) or formatted text
	CapturedAt  time.Time
}

// PreviewStore captures notifications in memory for dev-mode browser viewing.
// It implements the Notifier interface as a wrapper that captures instead of sending.
type PreviewStore struct {
	mu      sync.Mutex
	entries []PreviewEntry
	nextID  int
}

// NewPreviewStore creates a new preview store.
func NewPreviewStore() *PreviewStore {
	return &PreviewStore{}
}

// Capture records a notification entry for later viewing.
func (s *PreviewStore) Capture(channelName, channelType string, event BuildEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++

	html := ""
	// For email types, render the HTML template
	switch channelType {
	case "email_smtp", "email_sendgrid", "email_mailgun":
		rendered, err := renderEmailHTML(event)
		if err == nil {
			html = rendered
		}
	}

	s.entries = append(s.entries, PreviewEntry{
		ID:          s.nextID,
		ChannelName: channelName,
		ChannelType: channelType,
		Event:       event,
		HTML:        html,
		CapturedAt:  time.Now(),
	})

	// Keep only the last 50 entries
	if len(s.entries) > 50 {
		s.entries = s.entries[len(s.entries)-50:]
	}
}

// List returns all captured entries, newest first.
func (s *PreviewStore) List() []PreviewEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]PreviewEntry, len(s.entries))
	for i, e := range s.entries {
		result[len(s.entries)-1-i] = e
	}
	return result
}

// Get returns a single entry by ID.
func (s *PreviewStore) Get(id int) (PreviewEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.entries {
		if e.ID == id {
			return e, true
		}
	}
	return PreviewEntry{}, false
}

// Clear removes all entries.
func (s *PreviewStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
}

// previewNotifier wraps any Notifier and captures instead of sending.
type previewNotifier struct {
	store       *PreviewStore
	channelName string
	channelType string
}

// Send captures the notification in the preview store instead of sending.
func (n *previewNotifier) Send(_ context.Context, event BuildEvent) error {
	n.store.Capture(n.channelName, n.channelType, event)
	return nil
}
