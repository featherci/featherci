package git

import (
	"context"
	"fmt"

	"github.com/featherci/featherci/internal/models"
)

// ProjectUserGetter is a narrow interface for retrieving users associated with a project.
type ProjectUserGetter interface {
	GetUsersForProject(ctx context.Context, projectID int64) ([]*models.User, error)
}

// TokenResolver resolves access tokens for projects.
type TokenResolver struct {
	projectUsers ProjectUserGetter
}

// NewTokenResolver creates a new TokenResolver.
func NewTokenResolver(projectUsers ProjectUserGetter) *TokenResolver {
	return &TokenResolver{projectUsers: projectUsers}
}

// ResolveToken finds an access token for the given project by looking up project users.
func (r *TokenResolver) ResolveToken(ctx context.Context, projectID int64) (string, error) {
	users, err := r.projectUsers.GetUsersForProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get project users: %w", err)
	}

	for _, u := range users {
		if u.AccessToken != "" {
			return u.AccessToken, nil
		}
	}

	return "", fmt.Errorf("no access token found for project %d", projectID)
}
