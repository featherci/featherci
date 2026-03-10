package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GitHubWebhook creates and deletes webhooks via the GitHub API.
type GitHubWebhook struct {
	// BaseURL allows overriding for GitHub Enterprise. Defaults to https://api.github.com.
	BaseURL string
}

func (g *GitHubWebhook) baseURL() string {
	if g.BaseURL != "" {
		return g.BaseURL
	}
	return "https://api.github.com"
}

// CreateWebhook creates a webhook on a GitHub repository.
// Returns the webhook ID as a string.
func (g *GitHubWebhook) CreateWebhook(ctx context.Context, token, repoFullName, webhookURL, secret string) (string, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/hooks", g.baseURL(), repoFullName)

	payload := map[string]interface{}{
		"name":   "web",
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
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST hooks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return fmt.Sprintf("%d", result.ID), nil
}

// DeleteWebhook deletes a webhook from a GitHub repository.
func (g *GitHubWebhook) DeleteWebhook(ctx context.Context, token, repoFullName, webhookID string) error {
	apiURL := fmt.Sprintf("%s/repos/%s/hooks/%s", g.baseURL(), repoFullName, webhookID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE hook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	return nil
}
