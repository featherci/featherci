package notify

import (
	"context"
	"fmt"
	"strings"
)

// SendgridNotifier sends email notifications via the SendGrid HTTP API.
type SendgridNotifier struct {
	apiKey string
	from   string
	to     []string
}

// NewSendgridNotifier creates a new SendGrid notifier from config.
func NewSendgridNotifier(config map[string]string) (*SendgridNotifier, error) {
	apiKey := config["api_key"]
	from := config["from"]
	to := config["to"]

	if apiKey == "" {
		return nil, fmt.Errorf("SendGrid api_key is required")
	}
	if from == "" {
		return nil, fmt.Errorf("SendGrid from address is required")
	}
	if to == "" {
		return nil, fmt.Errorf("SendGrid to address is required")
	}

	recipients := strings.Split(to, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	return &SendgridNotifier{apiKey: apiKey, from: from, to: recipients}, nil
}

// Send sends a build notification email via the SendGrid v3 API.
func (n *SendgridNotifier) Send(ctx context.Context, event BuildEvent) error {
	subject := event.EmailSubject()

	body, err := renderEmailHTML(event)
	if err != nil {
		return fmt.Errorf("rendering email: %w", err)
	}

	// Build personalizations for each recipient
	tos := make([]map[string]string, len(n.to))
	for i, addr := range n.to {
		tos[i] = map[string]string{"email": addr}
	}

	payload := map[string]any{
		"personalizations": []map[string]any{
			{"to": tos},
		},
		"from":    map[string]string{"email": n.from},
		"subject": subject,
		"content": []map[string]string{
			{"type": "text/html", "value": body},
		},
	}

	req, err := newJSONRequest(ctx, "https://api.sendgrid.com/v3/mail/send", payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+n.apiKey)

	return doRequest(req, "SendGrid")
}
