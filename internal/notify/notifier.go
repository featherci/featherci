package notify

import (
	"context"
	"fmt"
)

// Notifier sends build notifications via a specific channel type.
type Notifier interface {
	Send(ctx context.Context, event BuildEvent) error
}

// NewNotifier creates a Notifier for the given channel type and config.
func NewNotifier(channelType string, config map[string]string) (Notifier, error) {
	switch channelType {
	case "email_smtp":
		return NewSMTPNotifier(config)
	case "email_sendgrid":
		return NewSendgridNotifier(config)
	case "email_mailgun":
		return NewMailgunNotifier(config)
	case "slack":
		return NewSlackNotifier(config)
	case "discord":
		return NewDiscordNotifier(config)
	case "pushover":
		return NewPushoverNotifier(config)
	default:
		return nil, fmt.Errorf("unknown notification channel type: %s", channelType)
	}
}

// ChannelTypes returns the list of supported channel type keys.
func ChannelTypes() []string {
	return []string{"email_smtp", "email_sendgrid", "email_mailgun", "slack", "discord", "pushover"}
}

// ChannelTypeLabel returns a human-readable label for a channel type.
func ChannelTypeLabel(t string) string {
	switch t {
	case "email_smtp":
		return "Email (SMTP)"
	case "email_sendgrid":
		return "Email (SendGrid)"
	case "email_mailgun":
		return "Email (Mailgun)"
	case "slack":
		return "Slack"
	case "discord":
		return "Discord"
	case "pushover":
		return "Pushover"
	default:
		return t
	}
}
