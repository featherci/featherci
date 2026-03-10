// Package config handles loading and validating FeatherCI configuration.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Mode defines the operating mode of the FeatherCI instance.
type Mode string

const (
	ModeStandalone Mode = "standalone" // Single instance (master + worker)
	ModeMaster     Mode = "master"     // Accepts webhooks, distributes work
	ModeWorker     Mode = "worker"     // Polls master, executes builds
)

// Config holds all configuration for FeatherCI.
type Config struct {
	// Server settings
	BindAddr string // Address to bind HTTP server (e.g., ":8080")
	BaseURL  string // Public URL for OAuth callbacks and webhooks
	Mode     Mode   // Operating mode: standalone, master, worker
	DevMode  bool   // Development mode: skip OAuth, auto-login as admin

	// Database
	DatabasePath string // Path to SQLite database file

	// Security
	SecretKey    []byte   // 32-byte key for AES-256-GCM encryption
	Admins       []string // Usernames with admin privileges
	WorkerSecret string   // Shared secret for worker authentication

	// GitHub OAuth
	GitHubClientID     string
	GitHubClientSecret string

	// GitLab OAuth
	GitLabURL          string // Base URL (default: https://gitlab.com)
	GitLabClientID     string
	GitLabClientSecret string

	// Gitea/Forgejo OAuth
	GiteaURL          string // Base URL of Gitea/Forgejo instance
	GiteaClientID     string
	GiteaClientSecret string

	// Worker mode settings
	MasterURL string // URL of master instance (required for worker mode)

	// Cache
	CachePath string // Directory for build cache

	// Workspaces
	WorkspacePath string // Directory for build workspaces
}

// Load reads configuration from .env file (if present) and environment variables.
// Environment variables take precedence over .env file values.
func Load() (*Config, error) {
	// Load .env file if it exists (ignore error if file doesn't exist)
	_ = godotenv.Load()

	cfg := &Config{
		// Server defaults
		BindAddr: getEnv("FEATHERCI_BIND_ADDR", ":8080"),
		BaseURL:  getEnv("FEATHERCI_BASE_URL", "http://localhost:8080"),
		Mode:     Mode(getEnv("FEATHERCI_MODE", "standalone")),

		// Database default
		DatabasePath: getEnv("FEATHERCI_DATABASE_PATH", "./featherci.db"),

		// Security
		Admins:       parseList(getEnv("FEATHERCI_ADMINS", "")),
		WorkerSecret: getEnv("FEATHERCI_WORKER_SECRET", ""),

		// GitHub OAuth
		GitHubClientID:     getEnv("FEATHERCI_GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getEnv("FEATHERCI_GITHUB_CLIENT_SECRET", ""),

		// GitLab OAuth
		GitLabURL:          getEnv("FEATHERCI_GITLAB_URL", "https://gitlab.com"),
		GitLabClientID:     getEnv("FEATHERCI_GITLAB_CLIENT_ID", ""),
		GitLabClientSecret: getEnv("FEATHERCI_GITLAB_CLIENT_SECRET", ""),

		// Gitea OAuth
		GiteaURL:          getEnv("FEATHERCI_GITEA_URL", ""),
		GiteaClientID:     getEnv("FEATHERCI_GITEA_CLIENT_ID", ""),
		GiteaClientSecret: getEnv("FEATHERCI_GITEA_CLIENT_SECRET", ""),

		// Worker mode
		MasterURL: getEnv("FEATHERCI_MASTER_URL", ""),

		// Cache
		CachePath: getEnv("FEATHERCI_CACHE_PATH", "./cache"),

		// Workspaces
		WorkspacePath: getEnv("FEATHERCI_WORKSPACE_PATH", "./workspaces"),
	}

	// Decode base64 secret key
	secretKeyB64 := getEnv("FEATHERCI_SECRET_KEY", "")
	if secretKeyB64 != "" {
		key, err := base64.StdEncoding.DecodeString(secretKeyB64)
		if err != nil {
			return nil, fmt.Errorf("FEATHERCI_SECRET_KEY: invalid base64: %w", err)
		}
		cfg.SecretKey = key
	}

	return cfg, nil
}

// Validate checks that the configuration is valid and complete.
func (c *Config) Validate() error {
	var errs []string

	// In dev mode, generate a deterministic secret key if not provided
	if c.DevMode && len(c.SecretKey) == 0 {
		// Use a fixed dev key (DO NOT use in production!)
		c.SecretKey = []byte("featherci-dev-key-do-not-use!!__")
	}

	// Validate secret key
	if len(c.SecretKey) == 0 {
		errs = append(errs, "FEATHERCI_SECRET_KEY is required (generate with: featherci --generate-key)")
	} else if len(c.SecretKey) != 32 {
		errs = append(errs, fmt.Sprintf("FEATHERCI_SECRET_KEY must be 32 bytes, got %d", len(c.SecretKey)))
	}

	// Validate base URL
	if c.BaseURL == "" {
		errs = append(errs, "FEATHERCI_BASE_URL is required")
	} else if _, err := url.Parse(c.BaseURL); err != nil {
		errs = append(errs, fmt.Sprintf("FEATHERCI_BASE_URL is invalid: %v", err))
	}

	// Validate mode
	switch c.Mode {
	case ModeStandalone, ModeMaster, ModeWorker:
		// Valid
	default:
		errs = append(errs, fmt.Sprintf("FEATHERCI_MODE must be 'standalone', 'master', or 'worker', got '%s'", c.Mode))
	}

	// Validate worker mode requirements
	if c.Mode == ModeWorker {
		if c.MasterURL == "" {
			errs = append(errs, "FEATHERCI_MASTER_URL is required when mode is 'worker'")
		} else if _, err := url.Parse(c.MasterURL); err != nil {
			errs = append(errs, fmt.Sprintf("FEATHERCI_MASTER_URL is invalid: %v", err))
		}
		if c.WorkerSecret == "" {
			errs = append(errs, "FEATHERCI_WORKER_SECRET is required when mode is 'worker'")
		}
	}

	// Validate master mode requirements
	if c.Mode == ModeMaster {
		if c.WorkerSecret == "" {
			errs = append(errs, "FEATHERCI_WORKER_SECRET is required when mode is 'master'")
		}
	}

	// Skip OAuth and admin validation in dev mode
	if !c.DevMode {
		// Require at least one OAuth provider
		if !c.HasGitHubAuth() && !c.HasGitLabAuth() && !c.HasGiteaAuth() {
			errs = append(errs, "at least one OAuth provider must be configured (GitHub, GitLab, or Gitea)")
		}

		// Validate GitLab URL if GitLab auth is configured
		if c.HasGitLabAuth() {
			if _, err := url.Parse(c.GitLabURL); err != nil {
				errs = append(errs, fmt.Sprintf("FEATHERCI_GITLAB_URL is invalid: %v", err))
			}
		}

		// Validate Gitea URL if Gitea auth is configured
		if c.HasGiteaAuth() {
			if c.GiteaURL == "" {
				errs = append(errs, "FEATHERCI_GITEA_URL is required when Gitea OAuth is configured")
			} else if _, err := url.Parse(c.GiteaURL); err != nil {
				errs = append(errs, fmt.Sprintf("FEATHERCI_GITEA_URL is invalid: %v", err))
			}
		}

		// Require at least one admin
		if len(c.Admins) == 0 {
			errs = append(errs, "FEATHERCI_ADMINS is required (comma-separated list of admin usernames)")
		}
	}

	if len(errs) > 0 {
		return errors.New("configuration errors:\n  - " + strings.Join(errs, "\n  - "))
	}

	return nil
}

// HasGitHubAuth returns true if GitHub OAuth is configured.
func (c *Config) HasGitHubAuth() bool {
	return c.GitHubClientID != "" && c.GitHubClientSecret != ""
}

// HasGitLabAuth returns true if GitLab OAuth is configured.
func (c *Config) HasGitLabAuth() bool {
	return c.GitLabClientID != "" && c.GitLabClientSecret != ""
}

// HasGiteaAuth returns true if Gitea/Forgejo OAuth is configured.
func (c *Config) HasGiteaAuth() bool {
	return c.GiteaClientID != "" && c.GiteaClientSecret != ""
}

// EnabledProviders returns a list of configured OAuth provider names.
func (c *Config) EnabledProviders() []string {
	var providers []string
	if c.HasGitHubAuth() {
		providers = append(providers, "github")
	}
	if c.HasGitLabAuth() {
		providers = append(providers, "gitlab")
	}
	if c.HasGiteaAuth() {
		providers = append(providers, "gitea")
	}
	return providers
}

// IsAdmin returns true if the given username is an admin.
func (c *Config) IsAdmin(username string) bool {
	for _, admin := range c.Admins {
		if strings.EqualFold(admin, username) {
			return true
		}
	}
	return false
}

// getEnv returns the environment variable value or the default if not set.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseList splits a comma-separated string into a slice, trimming whitespace.
func parseList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
