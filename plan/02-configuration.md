---
model: sonnet
---

# Step 02: Configuration System

## Objective
Implement a robust configuration system that loads settings from `.env` file (if present) and environment variables, with sensible defaults and validation.

## Tasks

### 2.0 Add dotenv Dependency
```bash
go get github.com/joho/godotenv
```

### 2.1 Create Configuration Struct
```go
type Config struct {
    // Server
    BindAddr    string
    BaseURL     string
    Mode        string // "master", "worker", "standalone"
    
    // Database
    DatabasePath string
    
    // Security
    SecretKey   []byte // 32 bytes for AES-256
    Admins      []string
    WorkerSecret string
    
    // GitHub OAuth
    GitHubClientID     string
    GitHubClientSecret string
    
    // GitLab OAuth
    GitLabClientID     string
    GitLabClientSecret string
    GitLabURL          string // For self-hosted GitLab
    
    // Gitea/Forgejo OAuth
    GiteaURL          string
    GiteaClientID     string
    GiteaClientSecret string
    
    // Worker mode
    MasterURL string
    
    // Cache
    CachePath string
}
```

### 2.2 Environment Variable Loading
- Use `os.Getenv` with fallbacks
- Support both `FEATHERCI_` prefix
- Parse comma-separated lists (admins)
- Decode base64 secret key

### 2.3 Validation
- Require `SecretKey` to be exactly 32 bytes
- Require `BaseURL` for OAuth callbacks
- Require at least one OAuth provider configured
- Require `MasterURL` when mode is "worker"
- Validate URLs are well-formed

### 2.4 Configuration Loading Function
```go
func Load() (*Config, error)
func (c *Config) Validate() error
func (c *Config) HasGitHubAuth() bool
func (c *Config) HasGitLabAuth() bool
func (c *Config) HasGiteaAuth() bool
func (c *Config) EnabledProviders() []string
```

### 2.5 Add Tests
- Test loading from environment
- Test validation rules
- Test default values
- Test base64 decoding of secret key

## Deliverables
- [ ] `internal/config/config.go` with full implementation
- [ ] `internal/config/config_test.go` with tests
- [ ] Environment variable documentation in comments
- [ ] Validation errors are descriptive

## Dependencies
- Step 01: Project initialization

## Estimated Effort
Small - Straightforward configuration loading
