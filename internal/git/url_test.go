package git

import (
	"testing"
)

func TestInjectToken(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		token    string
		provider string
		want     string
		wantErr  bool
	}{
		{
			name:     "github token injection",
			url:      "https://github.com/owner/repo.git",
			token:    "ghp_abc123",
			provider: "github",
			want:     "https://x-access-token:ghp_abc123@github.com/owner/repo.git",
		},
		{
			name:     "gitlab token injection",
			url:      "https://gitlab.com/owner/repo.git",
			token:    "glpat-abc123",
			provider: "gitlab",
			want:     "https://oauth2:glpat-abc123@gitlab.com/owner/repo.git",
		},
		{
			name:     "gitea token injection",
			url:      "https://gitea.example.com/owner/repo.git",
			token:    "tok123",
			provider: "gitea",
			want:     "https://oauth2:tok123@gitea.example.com/owner/repo.git",
		},
		{
			name:     "empty token returns URL unchanged",
			url:      "https://github.com/owner/repo.git",
			token:    "",
			provider: "github",
			want:     "https://github.com/owner/repo.git",
		},
		{
			name:     "non-HTTPS URL rejected",
			url:      "git@github.com:owner/repo.git",
			token:    "tok123",
			provider: "github",
			wantErr:  true,
		},
		{
			name:     "HTTP URL rejected",
			url:      "http://github.com/owner/repo.git",
			token:    "tok123",
			provider: "github",
			wantErr:  true,
		},
		{
			name:     "unsupported provider",
			url:      "https://example.com/owner/repo.git",
			token:    "tok123",
			provider: "bitbucket",
			wantErr:  true,
		},
		{
			name:     "malformed URL",
			url:      "://invalid",
			token:    "tok123",
			provider: "github",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InjectToken(tt.url, tt.token, tt.provider)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
