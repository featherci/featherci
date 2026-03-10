package worker

import (
	"context"
	"fmt"

	"github.com/featherci/featherci/internal/models"
)

// projectUserRepo is a subset of models.ProjectUserRepository for token resolution.
type projectUserRepo interface {
	GetUsersForProject(ctx context.Context, projectID int64) ([]*models.User, error)
}

// ProjectTokenSource resolves git tokens by finding a project user's access token.
type ProjectTokenSource struct {
	projectUsers projectUserRepo
}

// NewProjectTokenSource creates a new ProjectTokenSource.
func NewProjectTokenSource(projectUsers projectUserRepo) *ProjectTokenSource {
	return &ProjectTokenSource{projectUsers: projectUsers}
}

// TokenForProject returns an access token for cloning a project's repository.
// It picks the first user with access to the project and returns their token.
func (s *ProjectTokenSource) TokenForProject(ctx context.Context, projectID int64) (string, error) {
	users, err := s.projectUsers.GetUsersForProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get project users: %w", err)
	}
	if len(users) == 0 {
		return "", fmt.Errorf("no users found for project %d", projectID)
	}
	for _, u := range users {
		if u.AccessToken != "" {
			return u.AccessToken, nil
		}
	}
	return "", fmt.Errorf("no user with access token found for project %d", projectID)
}
