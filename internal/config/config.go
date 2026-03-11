// Package config handles loading and validating FeatherCI configuration.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// Default config file search paths, checked in order.
var defaultConfigPaths = []string{
	"/etc/featherci/config.yaml",
	"config.yaml",
}

// fileConfig is the YAML config file structure.
type fileConfig struct {
	BindAddr      string `yaml:"bind_addr"`
	BaseURL       string `yaml:"base_url"`
	Mode          string `yaml:"mode"`
	DatabasePath  string `yaml:"database_path"`
	SecretKey     string `yaml:"secret_key"`
	Admins        []string `yaml:"admins"`
	WorkerSecret  string `yaml:"worker_secret"`
	MasterURL     string `yaml:"master_url"`
	MaxConcurrent int    `yaml:"max_concurrent"`
	CachePath     string `yaml:"cache_path"`
	WorkspacePath string `yaml:"workspace_path"`

	GitHub struct {
		ClientID     string `yaml:"client_id"`
		ClientSecret string `yaml:"client_secret"`
	} `yaml:"github"`

	GitLab struct {
		URL          string `yaml:"url"`
		ClientID     string `yaml:"client_id"`
		ClientSecret string `yaml:"client_secret"`
	} `yaml:"gitlab"`

	Gitea struct {
		URL          string `yaml:"url"`
		ClientID     string `yaml:"client_id"`
		ClientSecret string `yaml:"client_secret"`
	} `yaml:"gitea"`
}

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
	MasterURL      string // URL of master instance (required for worker mode)
	MaxConcurrent  int    // Max concurrent build steps per worker (default: 2)

	// Cache
	CachePath string // Directory for build cache

	// Workspaces
	WorkspacePath string // Directory for build workspaces
}

// Load reads configuration from a YAML config file, .env file, and environment variables.
// Precedence (highest to lowest): environment variables > .env file > YAML config file > defaults.
// If configPath is empty, the default search paths are checked.
func Load(configPath string) (*Config, error) {
	// Load YAML config file (lowest precedence, sets defaults before env)
	fc := loadConfigFile(configPath)

	// Load .env file if it exists (ignore error if file doesn't exist)
	_ = godotenv.Load()

	cfg := &Config{
		// Server defaults
		BindAddr: getEnv("FEATHERCI_BIND_ADDR", firstNonEmpty(fc.BindAddr, ":8080")),
		BaseURL:  getEnv("FEATHERCI_BASE_URL", firstNonEmpty(fc.BaseURL, "http://localhost:8080")),
		Mode:     Mode(getEnv("FEATHERCI_MODE", firstNonEmpty(fc.Mode, "standalone"))),

		// Database default
		DatabasePath: getEnv("FEATHERCI_DATABASE_PATH", firstNonEmpty(fc.DatabasePath, "./featherci.db")),

		// Security
		Admins:       envOrYAMLList("FEATHERCI_ADMINS", fc.Admins),
		WorkerSecret: getEnv("FEATHERCI_WORKER_SECRET", fc.WorkerSecret),

		// GitHub OAuth
		GitHubClientID:     getEnv("FEATHERCI_GITHUB_CLIENT_ID", fc.GitHub.ClientID),
		GitHubClientSecret: getEnv("FEATHERCI_GITHUB_CLIENT_SECRET", fc.GitHub.ClientSecret),

		// GitLab OAuth
		GitLabURL:          getEnv("FEATHERCI_GITLAB_URL", firstNonEmpty(fc.GitLab.URL, "https://gitlab.com")),
		GitLabClientID:     getEnv("FEATHERCI_GITLAB_CLIENT_ID", fc.GitLab.ClientID),
		GitLabClientSecret: getEnv("FEATHERCI_GITLAB_CLIENT_SECRET", fc.GitLab.ClientSecret),

		// Gitea OAuth
		GiteaURL:          getEnv("FEATHERCI_GITEA_URL", fc.Gitea.URL),
		GiteaClientID:     getEnv("FEATHERCI_GITEA_CLIENT_ID", fc.Gitea.ClientID),
		GiteaClientSecret: getEnv("FEATHERCI_GITEA_CLIENT_SECRET", fc.Gitea.ClientSecret),

		// Worker mode
		MasterURL:     getEnv("FEATHERCI_MASTER_URL", fc.MasterURL),
		MaxConcurrent: getEnvInt("FEATHERCI_MAX_CONCURRENT", firstNonZero(fc.MaxConcurrent, 2)),

		// Cache
		CachePath: getEnv("FEATHERCI_CACHE_PATH", firstNonEmpty(fc.CachePath, "./cache")),

		// Workspaces
		WorkspacePath: getEnv("FEATHERCI_WORKSPACE_PATH", firstNonEmpty(fc.WorkspacePath, "./workspaces")),
	}

	// Decode base64 secret key (env var takes precedence over YAML)
	secretKeyB64 := getEnv("FEATHERCI_SECRET_KEY", fc.SecretKey)
	if secretKeyB64 != "" {
		key, err := base64.StdEncoding.DecodeString(secretKeyB64)
		if err != nil {
			return nil, fmt.Errorf("FEATHERCI_SECRET_KEY: invalid base64: %w", err)
		}
		cfg.SecretKey = key
	}

	return cfg, nil
}

// loadConfigFile reads a YAML config file. If path is empty, default locations are searched.
// Returns a zero-value fileConfig if no file is found.
func loadConfigFile(path string) fileConfig {
	var fc fileConfig

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return fc
		}
		_ = yaml.Unmarshal(data, &fc)
		return fc
	}

	// Check FEATHERCI_CONFIG env var
	if envPath := os.Getenv("FEATHERCI_CONFIG"); envPath != "" {
		data, err := os.ReadFile(envPath)
		if err != nil {
			return fc
		}
		_ = yaml.Unmarshal(data, &fc)
		return fc
	}

	// Search default paths
	for _, p := range defaultConfigPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		_ = yaml.Unmarshal(data, &fc)
		return fc
	}

	return fc
}

// envOrYAMLList returns the env var parsed as a comma-separated list,
// or falls back to the YAML list value.
func envOrYAMLList(envKey string, yamlVal []string) []string {
	if v := os.Getenv(envKey); v != "" {
		return parseList(v)
	}
	return yamlVal
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// firstNonZero returns the first non-zero int.
func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

// Validate checks that the configuration is valid and complete.
func (c *Config) Validate() error {
	var errs []string

	// In dev mode, generate a deterministic secret key if not provided
	if c.DevMode && len(c.SecretKey) == 0 {
		// Use a fixed dev key (DO NOT use in production!)
		c.SecretKey = []byte("featherci-dev-key-do-not-use!!__")
	}

	// Worker mode doesn't need secret key, base URL, or OAuth
	if c.Mode != ModeWorker {
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

	// Skip OAuth and admin validation in dev mode or worker mode
	if !c.DevMode && c.Mode != ModeWorker {
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

// getEnvInt returns the environment variable as an int, or the default if not set or invalid.
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			return n
		}
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
