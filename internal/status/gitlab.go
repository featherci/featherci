package status

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GitLabPoster posts commit statuses to the GitLab API.
type GitLabPoster struct {
	BaseURL string // e.g. "https://gitlab.com"
}

func (p *GitLabPoster) baseURL() string {
	if p.BaseURL != "" {
		return strings.TrimRight(p.BaseURL, "/")
	}
	return "https://gitlab.com"
}

// mapGitLabState maps a normalized CommitState to a GitLab pipeline status.
// GitLab uses "failed" instead of "failure" and "canceled" instead of "cancelled".
func mapGitLabState(s CommitState) string {
	switch s {
	case StatePending:
		return "pending"
	case StateRunning:
		return "running"
	case StateSuccess:
		return "success"
	case StateFailure:
		return "failed"
	case StateCancelled:
		return "canceled"
	default:
		return "pending"
	}
}

// PostStatus posts a commit status to the GitLab API.
func (p *GitLabPoster) PostStatus(ctx context.Context, token string, opts StatusOptions) error {
	// GitLab uses URL-encoded project path instead of separate owner/repo
	projectPath := url.PathEscape(opts.Owner + "/" + opts.Repo)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/statuses/%s", p.baseURL(), projectPath, opts.CommitSHA)

	body, err := json.Marshal(map[string]string{
		"state":       mapGitLabState(opts.State),
		"target_url":  opts.TargetURL,
		"description": description(opts.State),
		"name":        opts.Context,
	})
	if err != nil {
		return fmt.Errorf("marshal gitlab status: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create gitlab status request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post gitlab status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("gitlab status API returned %d", resp.StatusCode)
	}
	return nil
}
