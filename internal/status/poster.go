// Package status posts commit statuses to git providers (GitHub, GitLab, Gitea)
// so build results appear directly on PRs and commit pages.
package status

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/featherci/featherci/internal/config"
	"github.com/featherci/featherci/internal/models"
)

// CommitState represents a normalized commit status state.
type CommitState string

const (
	StatePending   CommitState = "pending"
	StateRunning   CommitState = "running"
	StateSuccess   CommitState = "success"
	StateFailure   CommitState = "failure"
	StateCancelled CommitState = "cancelled"
)

// StatusOptions describes a commit status to post.
type StatusOptions struct {
	Owner     string
	Repo      string
	CommitSHA string
	State     CommitState
	TargetURL string
	Context   string
}

// StatusPoster posts commit statuses to a specific provider.
type StatusPoster interface {
	PostStatus(ctx context.Context, token string, opts StatusOptions) error
}

// tokenSource provides access tokens for git provider API calls.
type tokenSource interface {
	TokenForProject(ctx context.Context, projectID int64) (string, error)
}

// StatusService maps build statuses to commit states and posts them via
// the appropriate provider poster.
type StatusService struct {
	posters map[string]StatusPoster
	tokens  tokenSource
	baseURL string
	logger  *slog.Logger
}

// NewStatusService creates a StatusService with provider posters registered.
func NewStatusService(cfg *config.Config, tokens tokenSource, logger *slog.Logger) *StatusService {
	if logger == nil {
		logger = slog.Default()
	}
	posters := map[string]StatusPoster{
		"github": &GitHubPoster{},
		"gitlab": &GitLabPoster{BaseURL: cfg.GitLabURL},
		"gitea":  &GiteaPoster{BaseURL: cfg.GiteaURL},
	}
	return &StatusService{
		posters: posters,
		tokens:  tokens,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		logger:  logger,
	}
}

// PostBuildStatus posts a commit status for the given build.
func (s *StatusService) PostBuildStatus(ctx context.Context, project *models.Project, build *models.Build) {
	poster, ok := s.posters[project.Provider]
	if !ok {
		s.logger.Warn("no status poster for provider", "provider", project.Provider)
		return
	}

	token, err := s.tokens.TokenForProject(ctx, project.ID)
	if err != nil {
		s.logger.Error("failed to get token for status post", "project_id", project.ID, "error", err)
		return
	}

	owner, repo := splitFullName(project.FullName)

	opts := StatusOptions{
		Owner:     owner,
		Repo:      repo,
		CommitSHA: build.CommitSHA,
		State:     mapBuildStatus(build.Status),
		TargetURL: fmt.Sprintf("%s/projects/%d/builds/%d", s.baseURL, project.ID, build.BuildNumber),
		Context:   "featherci",
	}

	if err := poster.PostStatus(ctx, token, opts); err != nil {
		s.logger.Error("failed to post commit status",
			"provider", project.Provider,
			"project_id", project.ID,
			"build_id", build.ID,
			"state", opts.State,
			"error", err,
		)
		return
	}

	s.logger.Debug("posted commit status",
		"provider", project.Provider,
		"commit", build.CommitSHA[:8],
		"state", opts.State,
	)
}

// mapBuildStatus converts a FeatherCI build status to a normalized commit state.
func mapBuildStatus(s models.BuildStatus) CommitState {
	switch s {
	case models.BuildStatusPending:
		return StatePending
	case models.BuildStatusRunning:
		return StateRunning
	case models.BuildStatusSuccess:
		return StateSuccess
	case models.BuildStatusFailure:
		return StateFailure
	case models.BuildStatusCancelled:
		return StateCancelled
	default:
		return StatePending
	}
}

// splitFullName splits "owner/repo" into owner and repo parts.
func splitFullName(fullName string) (string, string) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return fullName, ""
	}
	return parts[0], parts[1]
}

// description returns a human-readable description for the commit state.
func description(state CommitState) string {
	switch state {
	case StatePending:
		return "Build is pending"
	case StateRunning:
		return "Build is running"
	case StateSuccess:
		return "Build succeeded"
	case StateFailure:
		return "Build failed"
	case StateCancelled:
		return "Build was cancelled"
	default:
		return "Build status unknown"
	}
}
