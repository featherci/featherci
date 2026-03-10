// Package gitclient provides provider API clients for fetching repository content.
package gitclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

// FileContentFetcher fetches file content from git provider APIs without cloning.
type FileContentFetcher struct {
	gitLabURL string
	giteaURL  string
}

// NewFileContentFetcher creates a new FileContentFetcher.
// gitLabURL and giteaURL are the base URLs for self-hosted instances (e.g., "https://gitlab.com").
func NewFileContentFetcher(gitLabURL, giteaURL string) *FileContentFetcher {
	return &FileContentFetcher{
		gitLabURL: strings.TrimRight(gitLabURL, "/"),
		giteaURL:  strings.TrimRight(giteaURL, "/"),
	}
}

// GetFileContent fetches a file from a repository via the provider's API.
func (f *FileContentFetcher) GetFileContent(ctx context.Context, provider, token, repoFullName, filePath, ref string) ([]byte, error) {
	switch provider {
	case "github":
		return f.getGitHubFile(ctx, token, repoFullName, filePath, ref)
	case "gitlab":
		return f.getGitLabFile(ctx, token, repoFullName, filePath, ref)
	case "gitea":
		return f.getGiteaFile(ctx, token, repoFullName, filePath, ref)
	default:
		return nil, fmt.Errorf("unsupported provider: %q", provider)
	}
}

func (f *FileContentFetcher) getGitHubFile(ctx context.Context, token, repoFullName, filePath, ref string) ([]byte, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s?ref=%s",
		repoFullName, url.PathEscape(filePath), url.QueryEscape(ref))
	return f.fetchAndDecode(ctx, token, apiURL)
}

func (f *FileContentFetcher) getGitLabFile(ctx context.Context, token, repoFullName, filePath, ref string) ([]byte, error) {
	encodedProject := url.PathEscape(repoFullName)
	encodedPath := url.PathEscape(filePath)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s?ref=%s",
		f.gitLabURL, encodedProject, encodedPath, url.QueryEscape(ref))
	return f.fetchAndDecode(ctx, token, apiURL)
}

func (f *FileContentFetcher) getGiteaFile(ctx context.Context, token, repoFullName, filePath, ref string) ([]byte, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/contents/%s?ref=%s",
		f.giteaURL, repoFullName, url.PathEscape(filePath), url.QueryEscape(ref))
	return f.fetchAndDecode(ctx, token, apiURL)
}

// BranchHead contains the latest commit info for a branch.
type BranchHead struct {
	CommitSHA     string
	CommitMessage string
	CommitAuthor  string
}

// GetBranchHead returns the latest commit SHA, message, and author for a branch.
func (f *FileContentFetcher) GetBranchHead(ctx context.Context, provider, token, repoFullName, branch string) (*BranchHead, error) {
	switch provider {
	case "github":
		return f.getGitHubBranchHead(ctx, token, repoFullName, branch)
	case "gitlab":
		return f.getGitLabBranchHead(ctx, token, repoFullName, branch)
	case "gitea":
		return f.getGiteaBranchHead(ctx, token, repoFullName, branch)
	default:
		return nil, fmt.Errorf("unsupported provider: %q", provider)
	}
}

func (f *FileContentFetcher) getGitHubBranchHead(ctx context.Context, token, repoFullName, branch string) (*BranchHead, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/branches/%s", repoFullName, url.PathEscape(branch))

	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Commit struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
				Author  struct {
					Name string `json:"name"`
				} `json:"author"`
			} `json:"commit"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &BranchHead{
		CommitSHA:     result.Commit.SHA,
		CommitMessage: result.Commit.Commit.Message,
		CommitAuthor:  result.Commit.Commit.Author.Name,
	}, nil
}

func (f *FileContentFetcher) getGitLabBranchHead(ctx context.Context, token, repoFullName, branch string) (*BranchHead, error) {
	encodedProject := url.PathEscape(repoFullName)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches/%s", f.gitLabURL, encodedProject, url.PathEscape(branch))

	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Commit struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  string `json:"author_name"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &BranchHead{
		CommitSHA:     result.Commit.ID,
		CommitMessage: result.Commit.Message,
		CommitAuthor:  result.Commit.Author,
	}, nil
}

func (f *FileContentFetcher) getGiteaBranchHead(ctx context.Context, token, repoFullName, branch string) (*BranchHead, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/branches/%s", f.giteaURL, repoFullName, url.PathEscape(branch))

	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gitea API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Commit struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &BranchHead{
		CommitSHA:     result.Commit.ID,
		CommitMessage: result.Commit.Message,
		CommitAuthor:  result.Commit.Author.Name,
	}, nil
}

func (f *FileContentFetcher) fetchAndDecode(ctx context.Context, token, apiURL string) ([]byte, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: token,
	}))

	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("file not found")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode API response: %w", err)
	}

	// GitHub returns content with newlines in base64; strip them
	cleaned := strings.ReplaceAll(result.Content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 content: %w", err)
	}

	return decoded, nil
}
