package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// PushoverNotifier sends notifications via the Pushover API.
type PushoverNotifier struct {
	appToken string
	userKey  string
}

// NewPushoverNotifier creates a new Pushover notifier from config.
func NewPushoverNotifier(config map[string]string) (*PushoverNotifier, error) {
	appToken := config["app_token"]
	userKey := config["user_key"]
	if appToken == "" {
		return nil, fmt.Errorf("Pushover app_token is required")
	}
	if userKey == "" {
		return nil, fmt.Errorf("Pushover user_key is required")
	}
	return &PushoverNotifier{appToken: appToken, userKey: userKey}, nil
}

// Send sends a push notification via Pushover.
func (n *PushoverNotifier) Send(ctx context.Context, event BuildEvent) error {
	title := fmt.Sprintf("%s %s #%d %s",
		event.StatusEmoji(), event.ProjectName, event.BuildNumber, event.Status)

	message := fmt.Sprintf("Branch: %s\nCommit: %s\nAuthor: %s\nDuration: %s\n%s",
		event.Branch, event.ShortSHA(), event.CommitAuthor, event.DurationString(), event.CommitMessage)

	priority := "0"
	if !event.IsSuccess() && !event.IsCancelled() {
		priority = "1" // high priority for failures only
	}

	form := url.Values{
		"token":     {n.appToken},
		"user":      {n.userKey},
		"title":     {title},
		"message":   {message},
		"priority":  {priority},
		"url":       {event.BuildURL},
		"url_title": {"View Build"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.pushover.net/1/messages.json",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending pushover notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("pushover API returned status %d", resp.StatusCode)
	}
	return nil
}
