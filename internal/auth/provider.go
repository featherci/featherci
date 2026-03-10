// Package auth provides OAuth authentication for multiple providers.
package auth

import (
	"context"

	"golang.org/x/oauth2"
)

// Provider defines the interface for OAuth providers.
type Provider interface {
	// Name returns the provider identifier (e.g., "github", "gitlab", "gitea").
	Name() string

	// AuthCodeURL returns the URL to redirect the user to for authentication.
	AuthCodeURL(state string) string

	// Exchange exchanges the authorization code for an access token.
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)

	// GetUser retrieves the authenticated user's information.
	GetUser(ctx context.Context, token *oauth2.Token) (*UserInfo, error)

	// GetRepositories retrieves the user's accessible repositories.
	GetRepositories(ctx context.Context, token *oauth2.Token) ([]Repository, error)

	// RefreshToken refreshes an expired access token.
	RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error)
}

// UserInfo contains information about an authenticated user.
type UserInfo struct {
	ID        string // Provider-specific user ID
	Username  string // Login/username
	Email     string // Email address (may be empty)
	AvatarURL string // Profile picture URL
}

// Repository contains information about a git repository.
type Repository struct {
	ID        string // Provider-specific repository ID
	FullName  string // Full name (namespace/name)
	Namespace string // Owner/organization name
	Name      string // Repository name
	CloneURL  string // HTTPS clone URL
	SSHURL    string // SSH clone URL
	Private   bool   // Whether the repository is private
	Admin     bool   // Whether the user has admin access
	Push      bool   // Whether the user has push access
}
