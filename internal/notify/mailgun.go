package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// MailgunNotifier sends email notifications via the Mailgun HTTP API.
type MailgunNotifier struct {
	apiKey string
	domain string
	from   string
	to     []string
}

// NewMailgunNotifier creates a new Mailgun notifier from config.
func NewMailgunNotifier(config map[string]string) (*MailgunNotifier, error) {
	apiKey := config["api_key"]
	domain := config["domain"]
	from := config["from"]
	to := config["to"]

	if apiKey == "" {
		return nil, fmt.Errorf("Mailgun api_key is required")
	}
	if domain == "" {
		return nil, fmt.Errorf("Mailgun domain is required")
	}
	if from == "" {
		return nil, fmt.Errorf("Mailgun from address is required")
	}
	if to == "" {
		return nil, fmt.Errorf("Mailgun to address is required")
	}

	recipients := strings.Split(to, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	return &MailgunNotifier{apiKey: apiKey, domain: domain, from: from, to: recipients}, nil
}

// Send sends a build notification email via the Mailgun API.
func (n *MailgunNotifier) Send(ctx context.Context, event BuildEvent) error {
	subject := event.EmailSubject()

	body, err := renderEmailHTML(event)
	if err != nil {
		return fmt.Errorf("rendering email: %w", err)
	}

	form := url.Values{
		"from":    {n.from},
		"to":      {strings.Join(n.to, ",")},
		"subject": {subject},
		"html":    {body},
	}

	apiURL := fmt.Sprintf("https://api.mailgun.net/v3/%s/messages", n.domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("api", n.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending mailgun request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("Mailgun API returned status %d", resp.StatusCode)
	}
	return nil
}
