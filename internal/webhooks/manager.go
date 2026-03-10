// Package webhooks registers and unregisters webhooks on git providers.
package webhooks

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/featherci/featherci/internal/config"
)

// WebhookProvider can create and delete webhooks on a git hosting provider.
type WebhookProvider interface {
	CreateWebhook(ctx context.Context, token, repoFullName, webhookURL, secret string) (string, error)
	DeleteWebhook(ctx context.Context, token, repoFullName, webhookID string) error
}

// Manager handles webhook registration across providers.
type Manager struct {
	providers map[string]WebhookProvider
	baseURL   string
	logger    *slog.Logger
}

// NewManager creates a Manager with provider clients registered.
func NewManager(cfg *config.Config, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	providers := map[string]WebhookProvider{
		"github": &GitHubWebhook{},
		"gitlab": &GitLabWebhook{BaseURL: cfg.GitLabURL},
		"gitea":  &GiteaWebhook{BaseURL: cfg.GiteaURL},
	}
	return &Manager{
		providers: providers,
		baseURL:   strings.TrimRight(cfg.BaseURL, "/"),
		logger:    logger,
	}
}

// ShouldRegister returns false if the base URL is localhost or empty,
// since webhooks can't be delivered to a local-only address.
func (m *Manager) ShouldRegister() bool {
	if m.baseURL == "" {
		return false
	}
	u, err := url.Parse(m.baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host != "localhost" && host != "127.0.0.1" && host != "::1"
}

// WebhookURL returns the full webhook URL for a given provider.
func (m *Manager) WebhookURL(provider string) string {
	return fmt.Sprintf("%s/webhooks/%s", m.baseURL, provider)
}

// RegisterWebhook creates a webhook on the provider for the given repo.
// Returns the provider's webhook ID on success.
func (m *Manager) RegisterWebhook(ctx context.Context, provider, token, repoFullName, secret string) (string, error) {
	p, ok := m.providers[provider]
	if !ok {
		return "", fmt.Errorf("unsupported provider: %q", provider)
	}

	webhookURL := m.WebhookURL(provider)
	id, err := p.CreateWebhook(ctx, token, repoFullName, webhookURL, secret)
	if err != nil {
		return "", fmt.Errorf("create webhook for %s/%s: %w", provider, repoFullName, err)
	}

	m.logger.Info("registered webhook", "provider", provider, "repo", repoFullName, "webhook_id", id)
	return id, nil
}

// UnregisterWebhook deletes a webhook on the provider.
func (m *Manager) UnregisterWebhook(ctx context.Context, provider, token, repoFullName, webhookID string) error {
	p, ok := m.providers[provider]
	if !ok {
		return fmt.Errorf("unsupported provider: %q", provider)
	}

	if err := p.DeleteWebhook(ctx, token, repoFullName, webhookID); err != nil {
		return fmt.Errorf("delete webhook for %s/%s: %w", provider, repoFullName, err)
	}

	m.logger.Info("unregistered webhook", "provider", provider, "repo", repoFullName, "webhook_id", webhookID)
	return nil
}
