package notify

import (
	"context"
	"fmt"
)

// DiscordNotifier sends notifications to a Discord webhook.
type DiscordNotifier struct {
	webhookURL string
}

// NewDiscordNotifier creates a new Discord notifier from config.
func NewDiscordNotifier(config map[string]string) (*DiscordNotifier, error) {
	url := config["webhook_url"]
	if url == "" {
		return nil, fmt.Errorf("Discord webhook_url is required")
	}
	return &DiscordNotifier{webhookURL: url}, nil
}

// Send posts an embed message to Discord.
func (n *DiscordNotifier) Send(ctx context.Context, event BuildEvent) error {
	color := 0xdc2626 // red
	if event.IsSuccess() {
		color = 0x16a34a // green
	} else if event.IsCancelled() {
		color = 0x6b7280 // gray
	}

	payload := map[string]any{
		"embeds": []map[string]any{
			{
				"title":       fmt.Sprintf("%s Build #%d - %s", event.StatusEmoji(), event.BuildNumber, event.Status),
				"url":         event.BuildURL,
				"color":       color,
				"description": fmt.Sprintf("**%s**", event.ProjectName),
				"fields": []map[string]any{
					{"name": "Branch", "value": event.Branch, "inline": true},
					{"name": "Duration", "value": event.DurationString(), "inline": true},
					{"name": "Author", "value": event.CommitAuthor, "inline": true},
					{"name": "Commit", "value": fmt.Sprintf("`%s` %s", event.ShortSHA(), event.CommitMessage), "inline": false},
				},
				"footer": map[string]string{
					"text": "FeatherCI",
				},
			},
		},
	}

	return postJSON(ctx, n.webhookURL, payload)
}
