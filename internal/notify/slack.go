package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SlackNotifier sends notifications to a Slack webhook.
type SlackNotifier struct {
	webhookURL string
}

// NewSlackNotifier creates a new Slack notifier from config.
func NewSlackNotifier(config map[string]string) (*SlackNotifier, error) {
	url := config["webhook_url"]
	if url == "" {
		return nil, fmt.Errorf("Slack webhook_url is required")
	}
	return &SlackNotifier{webhookURL: url}, nil
}

// Send posts a rich-formatted message to Slack.
func (n *SlackNotifier) Send(ctx context.Context, event BuildEvent) error {
	color := "#dc2626" // red
	if event.IsSuccess() {
		color = "#16a34a" // green
	} else if event.IsCancelled() {
		color = "#6b7280" // gray
	}

	payload := map[string]any{
		"attachments": []map[string]any{
			{
				"color":      color,
				"fallback":   fmt.Sprintf("%s Build #%d %s", event.ProjectName, event.BuildNumber, event.Status),
				"pretext":    fmt.Sprintf("%s *%s* build #%d", event.StatusEmoji(), event.ProjectName, event.BuildNumber),
				"title":      fmt.Sprintf("Build #%d - %s", event.BuildNumber, event.Status),
				"title_link": event.BuildURL,
				"fields": []map[string]any{
					{"title": "Branch", "value": event.Branch, "short": true},
					{"title": "Duration", "value": event.DurationString(), "short": true},
					{"title": "Commit", "value": fmt.Sprintf("`%s` %s", event.ShortSHA(), event.CommitMessage), "short": false},
					{"title": "Author", "value": event.CommitAuthor, "short": true},
				},
				"mrkdwn_in": []string{"pretext", "fields"},
			},
		},
	}

	return postJSON(ctx, n.webhookURL, payload)
}

// postJSON posts a JSON payload to a URL.
func postJSON(ctx context.Context, url string, payload any) error {
	req, err := newJSONRequest(ctx, url, payload)
	if err != nil {
		return err
	}
	return doRequest(req, url)
}

// newJSONRequest creates an HTTP POST request with a JSON body.
func newJSONRequest(ctx context.Context, url string, payload any) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// doRequest executes an HTTP request and checks for error status codes.
func doRequest(req *http.Request, label string) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request to %s: %w", label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, label)
	}
	return nil
}
