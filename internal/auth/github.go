package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// GitHubProvider implements OAuth authentication for GitHub.
type GitHubProvider struct {
	config *oauth2.Config
}

// NewGitHubProvider creates a new GitHub OAuth provider.
func NewGitHubProvider(clientID, clientSecret, callbackURL string) *GitHubProvider {
	return &GitHubProvider{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  callbackURL,
			Scopes:       []string{"read:user", "user:email", "repo"},
			Endpoint:     github.Endpoint,
		},
	}
}

// Name returns "github".
func (p *GitHubProvider) Name() string {
	return "github"
}

// AuthCodeURL returns the URL to redirect the user to for authentication.
func (p *GitHubProvider) AuthCodeURL(state string) string {
	return p.config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// Exchange exchanges the authorization code for an access token.
func (p *GitHubProvider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return p.config.Exchange(ctx, code)
}

// GetUser retrieves the authenticated user's information from GitHub.
func (p *GitHubProvider) GetUser(ctx context.Context, token *oauth2.Token) (*UserInfo, error) {
	client := p.config.Client(ctx, token)

	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var user struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode user response: %w", err)
	}

	return &UserInfo{
		ID:        strconv.FormatInt(user.ID, 10),
		Username:  user.Login,
		Email:     user.Email,
		AvatarURL: user.AvatarURL,
	}, nil
}

// GetRepositories retrieves the user's accessible repositories from GitHub.
func (p *GitHubProvider) GetRepositories(ctx context.Context, token *oauth2.Token) ([]Repository, error) {
	client := p.config.Client(ctx, token)
	var repos []Repository
	page := 1

	for {
		url := fmt.Sprintf("https://api.github.com/user/repos?per_page=100&page=%d&sort=updated", page)
		resp, err := client.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to get repositories: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}

		var ghRepos []struct {
			ID       int64  `json:"id"`
			FullName string `json:"full_name"`
			Name     string `json:"name"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
			CloneURL    string `json:"clone_url"`
			SSHURL      string `json:"ssh_url"`
			Private     bool   `json:"private"`
			Permissions struct {
				Admin bool `json:"admin"`
				Push  bool `json:"push"`
				Pull  bool `json:"pull"`
			} `json:"permissions"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&ghRepos); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode repositories response: %w", err)
		}
		resp.Body.Close()

		if len(ghRepos) == 0 {
			break
		}

		for _, r := range ghRepos {
			repos = append(repos, Repository{
				ID:        strconv.FormatInt(r.ID, 10),
				FullName:  r.FullName,
				Namespace: r.Owner.Login,
				Name:      r.Name,
				CloneURL:  r.CloneURL,
				SSHURL:    r.SSHURL,
				Private:   r.Private,
				Admin:     r.Permissions.Admin,
				Push:      r.Permissions.Push,
			})
		}

		if len(ghRepos) < 100 {
			break
		}
		page++
	}

	return repos, nil
}

// RefreshToken refreshes an expired access token.
// Note: GitHub tokens don't expire by default, so this usually returns the same token.
func (p *GitHubProvider) RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	src := p.config.TokenSource(ctx, token)
	return src.Token()
}

// Ensure GitHubProvider implements Provider.
var _ Provider = (*GitHubProvider)(nil)
