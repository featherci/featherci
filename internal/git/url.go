// Package git provides git operations for cloning repositories and managing workspaces.
package git

import (
	"fmt"
	"net/url"
)

// InjectToken inserts authentication credentials into an HTTPS clone URL.
// For GitHub, it uses x-access-token:TOKEN. For GitLab and Gitea, it uses oauth2:TOKEN.
func InjectToken(cloneURL, token, provider string) (string, error) {
	if token == "" {
		return cloneURL, nil
	}

	u, err := url.Parse(cloneURL)
	if err != nil {
		return "", fmt.Errorf("invalid clone URL: %w", err)
	}

	if u.Scheme != "https" {
		return "", fmt.Errorf("token injection requires HTTPS URL, got %q", u.Scheme)
	}

	var username string
	switch provider {
	case "github":
		username = "x-access-token"
	case "gitlab", "gitea":
		username = "oauth2"
	default:
		return "", fmt.Errorf("unsupported provider: %q", provider)
	}

	u.User = url.UserPassword(username, token)
	return u.String(), nil
}
