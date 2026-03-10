package status

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// GitHubPoster posts commit statuses to the GitHub API.
type GitHubPoster struct {
	// BaseURL allows overriding for tests. Defaults to https://api.github.com.
	BaseURL string
}

func (p *GitHubPoster) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	return "https://api.github.com"
}

// mapGitHubState maps a normalized CommitState to a GitHub status state.
// GitHub has no "running" state, so running maps to "pending".
func mapGitHubState(s CommitState) string {
	switch s {
	case StatePending, StateRunning:
		return "pending"
	case StateSuccess:
		return "success"
	case StateFailure:
		return "failure"
	case StateCancelled:
		return "error"
	default:
		return "pending"
	}
}

// PostStatus posts a commit status to the GitHub API.
func (p *GitHubPoster) PostStatus(ctx context.Context, token string, opts StatusOptions) error {
	url := fmt.Sprintf("%s/repos/%s/%s/statuses/%s", p.baseURL(), opts.Owner, opts.Repo, opts.CommitSHA)

	body, err := json.Marshal(map[string]string{
		"state":       mapGitHubState(opts.State),
		"target_url":  opts.TargetURL,
		"description": description(opts.State),
		"context":     opts.Context,
	})
	if err != nil {
		return fmt.Errorf("marshal github status: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create github status request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post github status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("github status API returned %d", resp.StatusCode)
	}
	return nil
}
