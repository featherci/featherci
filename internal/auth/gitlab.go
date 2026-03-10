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

// GitLabProvider implements OAuth authentication for GitLab (including self-hosted).
type GitLabProvider struct {
	config  *oauth2.Config
	baseURL string
}

// NewGitLabProvider creates a new GitLab OAuth provider.
// baseURL should be "https://gitlab.com" for GitLab.com or the self-hosted instance URL.
func NewGitLabProvider(clientID, clientSecret, callbackURL, baseURL string) *GitLabProvider {
	// Ensure baseURL doesn't have trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &GitLabProvider{
		baseURL: baseURL,
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  callbackURL,
			Scopes:       []string{"read_user", "api", "read_repository"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  baseURL + "/oauth/authorize",
				TokenURL: baseURL + "/oauth/token",
			},
		},
	}
}

// Name returns "gitlab".
func (p *GitLabProvider) Name() string {
	return "gitlab"
}

// AuthCodeURL returns the URL to redirect the user to for authentication.
func (p *GitLabProvider) AuthCodeURL(state string) string {
	return p.config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// Exchange exchanges the authorization code for an access token.
func (p *GitLabProvider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return p.config.Exchange(ctx, code)
}

// GetUser retrieves the authenticated user's information from GitLab.
func (p *GitLabProvider) GetUser(ctx context.Context, token *oauth2.Token) (*UserInfo, error) {
	client := p.config.Client(ctx, token)

	resp, err := client.Get(p.baseURL + "/api/v4/user")
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API returned status %d", resp.StatusCode)
	}

	var user struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode user response: %w", err)
	}

	return &UserInfo{
		ID:        strconv.FormatInt(user.ID, 10),
		Username:  user.Username,
		Email:     user.Email,
		AvatarURL: user.AvatarURL,
	}, nil
}

// GetRepositories retrieves the user's accessible repositories from GitLab.
func (p *GitLabProvider) GetRepositories(ctx context.Context, token *oauth2.Token) ([]Repository, error) {
	client := p.config.Client(ctx, token)
	var repos []Repository
	page := 1

	for {
		url := fmt.Sprintf("%s/api/v4/projects?membership=true&per_page=100&page=%d&order_by=updated_at", p.baseURL, page)
		resp, err := client.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to get repositories: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitLab API returned status %d", resp.StatusCode)
		}

		var glProjects []struct {
			ID                int64  `json:"id"`
			PathWithNamespace string `json:"path_with_namespace"`
			Path              string `json:"path"`
			Namespace         struct {
				Path string `json:"path"`
			} `json:"namespace"`
			HTTPURLToRepo string `json:"http_url_to_repo"`
			SSHURLToRepo  string `json:"ssh_url_to_repo"`
			DefaultBranch string `json:"default_branch"`
			Visibility    string `json:"visibility"`
			Permissions   struct {
				ProjectAccess *struct {
					AccessLevel int `json:"access_level"`
				} `json:"project_access"`
				GroupAccess *struct {
					AccessLevel int `json:"access_level"`
				} `json:"group_access"`
			} `json:"permissions"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&glProjects); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode repositories response: %w", err)
		}
		resp.Body.Close()

		if len(glProjects) == 0 {
			break
		}

		for _, proj := range glProjects {
			// GitLab access levels: 10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner
			accessLevel := 0
			if proj.Permissions.ProjectAccess != nil {
				accessLevel = proj.Permissions.ProjectAccess.AccessLevel
			}
			if proj.Permissions.GroupAccess != nil && proj.Permissions.GroupAccess.AccessLevel > accessLevel {
				accessLevel = proj.Permissions.GroupAccess.AccessLevel
			}

			repos = append(repos, Repository{
				ID:            strconv.FormatInt(proj.ID, 10),
				FullName:      proj.PathWithNamespace,
				Namespace:     proj.Namespace.Path,
				Name:          proj.Path,
				CloneURL:      proj.HTTPURLToRepo,
				SSHURL:        proj.SSHURLToRepo,
				DefaultBranch: proj.DefaultBranch,
				Private:       proj.Visibility != "public",
				Admin:         accessLevel >= 40, // Maintainer or Owner
				Push:          accessLevel >= 30, // Developer or above
			})
		}

		if len(glProjects) < 100 {
			break
		}
		page++
	}

	return repos, nil
}

// RefreshToken refreshes an expired access token.
func (p *GitLabProvider) RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	src := p.config.TokenSource(ctx, token)
	return src.Token()
}

// Ensure GitLabProvider implements Provider.
var _ Provider = (*GitLabProvider)(nil)
