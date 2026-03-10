package status

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GiteaPoster posts commit statuses to the Gitea/Forgejo API.
type GiteaPoster struct {
	BaseURL string // e.g. "https://gitea.example.com"
}

func (p *GiteaPoster) baseURL() string {
	return strings.TrimRight(p.BaseURL, "/")
}

// mapGiteaState maps a normalized CommitState to a Gitea status state.
// Gitea uses the same states as GitHub.
func mapGiteaState(s CommitState) string {
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

// PostStatus posts a commit status to the Gitea API.
func (p *GiteaPoster) PostStatus(ctx context.Context, token string, opts StatusOptions) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/statuses/%s", p.baseURL(), opts.Owner, opts.Repo, opts.CommitSHA)

	body, err := json.Marshal(map[string]string{
		"state":       mapGiteaState(opts.State),
		"target_url":  opts.TargetURL,
		"description": description(opts.State),
		"context":     opts.Context,
	})
	if err != nil {
		return fmt.Errorf("marshal gitea status: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create gitea status request: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post gitea status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("gitea status API returned %d", resp.StatusCode)
	}
	return nil
}
