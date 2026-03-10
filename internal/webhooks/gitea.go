package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GiteaWebhook creates and deletes webhooks via the Gitea/Forgejo API.
type GiteaWebhook struct {
	BaseURL string // e.g. "https://gitea.example.com"
}

func (g *GiteaWebhook) baseURL() string {
	return strings.TrimRight(g.BaseURL, "/")
}

// CreateWebhook creates a webhook on a Gitea repository.
// Returns the webhook ID as a string.
func (g *GiteaWebhook) CreateWebhook(ctx context.Context, token, repoFullName, webhookURL, secret string) (string, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/hooks", g.baseURL(), repoFullName)

	payload := map[string]interface{}{
		"type":   "gitea",
		"active": true,
		"events": []string{"push", "pull_request"},
		"config": map[string]string{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST hooks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Gitea API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return fmt.Sprintf("%d", result.ID), nil
}

// DeleteWebhook deletes a webhook from a Gitea repository.
func (g *GiteaWebhook) DeleteWebhook(ctx context.Context, token, repoFullName, webhookID string) error {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/hooks/%s", g.baseURL(), repoFullName, webhookID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE hook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Gitea API returned %d", resp.StatusCode)
	}

	return nil
}
