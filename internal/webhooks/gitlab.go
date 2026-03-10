package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GitLabWebhook creates and deletes webhooks via the GitLab API.
type GitLabWebhook struct {
	BaseURL string // e.g. "https://gitlab.com"
}

func (g *GitLabWebhook) baseURL() string {
	if g.BaseURL != "" {
		return strings.TrimRight(g.BaseURL, "/")
	}
	return "https://gitlab.com"
}

// CreateWebhook creates a webhook on a GitLab project.
// Returns the webhook ID as a string.
func (g *GitLabWebhook) CreateWebhook(ctx context.Context, token, repoFullName, webhookURL, secret string) (string, error) {
	encodedProject := url.PathEscape(repoFullName)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/hooks", g.baseURL(), encodedProject)

	payload := map[string]interface{}{
		"url":                   webhookURL,
		"push_events":          true,
		"merge_requests_events": true,
		"token":                secret,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST hooks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitLab API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return fmt.Sprintf("%d", result.ID), nil
}

// DeleteWebhook deletes a webhook from a GitLab project.
func (g *GitLabWebhook) DeleteWebhook(ctx context.Context, token, repoFullName, webhookID string) error {
	encodedProject := url.PathEscape(repoFullName)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/hooks/%s", g.baseURL(), encodedProject, webhookID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE hook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitLab API returned %d", resp.StatusCode)
	}

	return nil
}
