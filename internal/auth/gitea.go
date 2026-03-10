package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/oauth2"
)

// GiteaProvider implements OAuth authentication for Gitea and Forgejo.
type GiteaProvider struct {
	config  *oauth2.Config
	baseURL string
}

// NewGiteaProvider creates a new Gitea/Forgejo OAuth provider.
// baseURL should be the Gitea/Forgejo instance URL (e.g., "https://gitea.example.com").
func NewGiteaProvider(clientID, clientSecret, callbackURL, baseURL string) *GiteaProvider {
	// Ensure baseURL doesn't have trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &GiteaProvider{
		baseURL: baseURL,
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  callbackURL,
			Scopes:       []string{"read:user", "read:repository", "write:repository"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  baseURL + "/login/oauth/authorize",
				TokenURL: baseURL + "/login/oauth/access_token",
			},
		},
	}
}

// Name returns "gitea".
func (p *GiteaProvider) Name() string {
	return "gitea"
}

// AuthCodeURL returns the URL to redirect the user to for authentication.
func (p *GiteaProvider) AuthCodeURL(state string) string {
	return p.config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// Exchange exchanges the authorization code for an access token.
func (p *GiteaProvider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return p.config.Exchange(ctx, code)
}

// GetUser retrieves the authenticated user's information from Gitea.
func (p *GiteaProvider) GetUser(ctx context.Context, token *oauth2.Token) (*UserInfo, error) {
	client := p.config.Client(ctx, token)

	resp, err := client.Get(p.baseURL + "/api/v1/user")
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gitea API returned status %d", resp.StatusCode)
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

// GetRepositories retrieves the user's accessible repositories from Gitea.
func (p *GiteaProvider) GetRepositories(ctx context.Context, token *oauth2.Token) ([]Repository, error) {
	client := p.config.Client(ctx, token)
	var repos []Repository
	page := 1

	for {
		url := fmt.Sprintf("%s/api/v1/user/repos?limit=50&page=%d", p.baseURL, page)
		resp, err := client.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to get repositories: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("Gitea API returned status %d", resp.StatusCode)
		}

		var giteaRepos []struct {
			ID       int64  `json:"id"`
			FullName string `json:"full_name"`
			Name     string `json:"name"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
			CloneURL      string `json:"clone_url"`
			SSHURL        string `json:"ssh_url"`
			DefaultBranch string `json:"default_branch"`
			Private       bool   `json:"private"`
			Permissions   struct {
				Admin bool `json:"admin"`
				Push  bool `json:"push"`
				Pull  bool `json:"pull"`
			} `json:"permissions"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&giteaRepos); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode repositories response: %w", err)
		}
		resp.Body.Close()

		if len(giteaRepos) == 0 {
			break
		}

		for _, r := range giteaRepos {
			repos = append(repos, Repository{
				ID:            strconv.FormatInt(r.ID, 10),
				FullName:      r.FullName,
				Namespace:     r.Owner.Login,
				Name:          r.Name,
				CloneURL:      r.CloneURL,
				SSHURL:        r.SSHURL,
				DefaultBranch: r.DefaultBranch,
				Private:       r.Private,
				Admin:         r.Permissions.Admin,
				Push:          r.Permissions.Push,
			})
		}

		if len(giteaRepos) < 50 {
			break
		}
		page++
	}

	return repos, nil
}

// RefreshToken refreshes an expired access token.
func (p *GiteaProvider) RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	src := p.config.TokenSource(ctx, token)
	return src.Token()
}

// Ensure GiteaProvider implements Provider.
var _ Provider = (*GiteaProvider)(nil)
