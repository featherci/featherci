package config

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

// validSecretKey is a 32-byte key encoded as base64 for testing.
var validSecretKey = base64.StdEncoding.EncodeToString(make([]byte, 32))

func TestLoad_Defaults(t *testing.T) {
	// Clear all env vars
	clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check defaults
	if cfg.BindAddr != ":8080" {
		t.Errorf("BindAddr = %q, want %q", cfg.BindAddr, ":8080")
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "http://localhost:8080")
	}
	if cfg.Mode != ModeStandalone {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeStandalone)
	}
	if cfg.DatabasePath != "./featherci.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, "./featherci.db")
	}
	if cfg.CachePath != "./cache" {
		t.Errorf("CachePath = %q, want %q", cfg.CachePath, "./cache")
	}
	if cfg.GitLabURL != "https://gitlab.com" {
		t.Errorf("GitLabURL = %q, want %q", cfg.GitLabURL, "https://gitlab.com")
	}
}

func TestLoad_FromEnv(t *testing.T) {
	clearEnv()

	os.Setenv("FEATHERCI_BIND_ADDR", ":9090")
	os.Setenv("FEATHERCI_BASE_URL", "https://ci.example.com")
	os.Setenv("FEATHERCI_MODE", "master")
	os.Setenv("FEATHERCI_DATABASE_PATH", "/data/ci.db")
	os.Setenv("FEATHERCI_SECRET_KEY", validSecretKey)
	os.Setenv("FEATHERCI_ADMINS", "alice, bob, charlie")
	os.Setenv("FEATHERCI_GITHUB_CLIENT_ID", "gh-client-id")
	os.Setenv("FEATHERCI_GITHUB_CLIENT_SECRET", "gh-client-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.BindAddr != ":9090" {
		t.Errorf("BindAddr = %q, want %q", cfg.BindAddr, ":9090")
	}
	if cfg.BaseURL != "https://ci.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://ci.example.com")
	}
	if cfg.Mode != ModeMaster {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeMaster)
	}
	if cfg.DatabasePath != "/data/ci.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, "/data/ci.db")
	}
	if len(cfg.SecretKey) != 32 {
		t.Errorf("SecretKey length = %d, want 32", len(cfg.SecretKey))
	}
	if len(cfg.Admins) != 3 {
		t.Errorf("Admins length = %d, want 3", len(cfg.Admins))
	}
	if cfg.Admins[0] != "alice" || cfg.Admins[1] != "bob" || cfg.Admins[2] != "charlie" {
		t.Errorf("Admins = %v, want [alice bob charlie]", cfg.Admins)
	}
}

func TestLoad_InvalidBase64(t *testing.T) {
	clearEnv()
	os.Setenv("FEATHERCI_SECRET_KEY", "not-valid-base64!!!")

	_, err := Load()
	if err == nil {
		t.Error("Load() expected error for invalid base64, got nil")
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		BindAddr:           ":8080",
		BaseURL:            "https://ci.example.com",
		Mode:               ModeStandalone,
		SecretKey:          make([]byte, 32),
		Admins:             []string{"admin"},
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_DevMode(t *testing.T) {
	// Dev mode should pass with minimal config
	cfg := &Config{
		BaseURL: "http://localhost:8080",
		Mode:    ModeStandalone,
		DevMode: true,
		// No SecretKey, Admins, or OAuth providers
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with DevMode error = %v, want nil", err)
	}

	// Should have generated a dev secret key
	if len(cfg.SecretKey) != 32 {
		t.Errorf("DevMode SecretKey length = %d, want 32", len(cfg.SecretKey))
	}
}

func TestValidate_MissingSecretKey(t *testing.T) {
	cfg := &Config{
		BaseURL:            "https://ci.example.com",
		Mode:               ModeStandalone,
		Admins:             []string{"admin"},
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() expected error for missing secret key, got nil")
	}
}

func TestValidate_WrongSecretKeyLength(t *testing.T) {
	cfg := &Config{
		BaseURL:            "https://ci.example.com",
		Mode:               ModeStandalone,
		SecretKey:          make([]byte, 16), // Wrong length
		Admins:             []string{"admin"},
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() expected error for wrong secret key length, got nil")
	}
}

func TestValidate_NoOAuthProvider(t *testing.T) {
	cfg := &Config{
		BaseURL:   "https://ci.example.com",
		Mode:      ModeStandalone,
		SecretKey: make([]byte, 32),
		Admins:    []string{"admin"},
		// No OAuth providers configured
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() expected error for no OAuth provider, got nil")
	}
}

func TestValidate_WorkerModeRequirements(t *testing.T) {
	cfg := &Config{
		BaseURL:            "https://ci.example.com",
		Mode:               ModeWorker,
		SecretKey:          make([]byte, 32),
		Admins:             []string{"admin"},
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
		// Missing MasterURL and WorkerSecret
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() expected error for worker mode without MasterURL, got nil")
	}

	// Add MasterURL but no WorkerSecret
	cfg.MasterURL = "https://master.example.com"
	err = cfg.Validate()
	if err == nil {
		t.Error("Validate() expected error for worker mode without WorkerSecret, got nil")
	}

	// Add WorkerSecret - should pass now
	cfg.WorkerSecret = "secret"
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_MasterModeRequirements(t *testing.T) {
	cfg := &Config{
		BaseURL:            "https://ci.example.com",
		Mode:               ModeMaster,
		SecretKey:          make([]byte, 32),
		Admins:             []string{"admin"},
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
		// Missing WorkerSecret
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() expected error for master mode without WorkerSecret, got nil")
	}

	cfg.WorkerSecret = "secret"
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_GiteaRequiresURL(t *testing.T) {
	cfg := &Config{
		BaseURL:           "https://ci.example.com",
		Mode:              ModeStandalone,
		SecretKey:         make([]byte, 32),
		Admins:            []string{"admin"},
		GiteaClientID:     "client-id",
		GiteaClientSecret: "client-secret",
		// Missing GiteaURL
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() expected error for Gitea without URL, got nil")
	}

	cfg.GiteaURL = "https://gitea.example.com"
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_InvalidMode(t *testing.T) {
	cfg := &Config{
		BaseURL:            "https://ci.example.com",
		Mode:               Mode("invalid"),
		SecretKey:          make([]byte, 32),
		Admins:             []string{"admin"},
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() expected error for invalid mode, got nil")
	}
}

func TestValidate_NoAdmins(t *testing.T) {
	cfg := &Config{
		BaseURL:            "https://ci.example.com",
		Mode:               ModeStandalone,
		SecretKey:          make([]byte, 32),
		Admins:             []string{}, // Empty
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() expected error for no admins, got nil")
	}
}

func TestHasOAuthProviders(t *testing.T) {
	cfg := &Config{}

	if cfg.HasGitHubAuth() {
		t.Error("HasGitHubAuth() = true, want false")
	}
	if cfg.HasGitLabAuth() {
		t.Error("HasGitLabAuth() = true, want false")
	}
	if cfg.HasGiteaAuth() {
		t.Error("HasGiteaAuth() = true, want false")
	}

	cfg.GitHubClientID = "id"
	cfg.GitHubClientSecret = "secret"
	if !cfg.HasGitHubAuth() {
		t.Error("HasGitHubAuth() = false, want true")
	}

	cfg.GitLabClientID = "id"
	cfg.GitLabClientSecret = "secret"
	if !cfg.HasGitLabAuth() {
		t.Error("HasGitLabAuth() = false, want true")
	}

	cfg.GiteaClientID = "id"
	cfg.GiteaClientSecret = "secret"
	if !cfg.HasGiteaAuth() {
		t.Error("HasGiteaAuth() = false, want true")
	}
}

func TestEnabledProviders(t *testing.T) {
	cfg := &Config{}

	providers := cfg.EnabledProviders()
	if len(providers) != 0 {
		t.Errorf("EnabledProviders() = %v, want empty", providers)
	}

	cfg.GitHubClientID = "id"
	cfg.GitHubClientSecret = "secret"
	cfg.GitLabClientID = "id"
	cfg.GitLabClientSecret = "secret"

	providers = cfg.EnabledProviders()
	if len(providers) != 2 {
		t.Errorf("EnabledProviders() length = %d, want 2", len(providers))
	}
	if providers[0] != "github" || providers[1] != "gitlab" {
		t.Errorf("EnabledProviders() = %v, want [github gitlab]", providers)
	}
}

func TestIsAdmin(t *testing.T) {
	cfg := &Config{
		Admins: []string{"Alice", "bob"},
	}

	tests := []struct {
		username string
		want     bool
	}{
		{"Alice", true},
		{"alice", true}, // Case insensitive
		{"ALICE", true},
		{"bob", true},
		{"Bob", true},
		{"charlie", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := cfg.IsAdmin(tt.username); got != tt.want {
			t.Errorf("IsAdmin(%q) = %v, want %v", tt.username, got, tt.want)
		}
	}
}

func TestParseList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"alice", []string{"alice"}},
		{"alice,bob", []string{"alice", "bob"}},
		{"alice, bob, charlie", []string{"alice", "bob", "charlie"}},
		{" alice , bob , ", []string{"alice", "bob"}},
		{",,,", nil},
	}

	for _, tt := range tests {
		got := parseList(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseList(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseList(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

// clearEnv removes all FEATHERCI_ environment variables.
func clearEnv() {
	for _, env := range os.Environ() {
		if len(env) > 10 && env[:10] == "FEATHERCI_" {
			key := env[:strings.Index(env, "=")]
			os.Unsetenv(key)
		}
	}
}
