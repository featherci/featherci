package git

import (
	"context"
	"fmt"
	"testing"

	"github.com/featherci/featherci/internal/models"
)

type mockProjectUserGetter struct {
	users []*models.User
	err   error
}

func (m *mockProjectUserGetter) GetUsersForProject(_ context.Context, _ int64) ([]*models.User, error) {
	return m.users, m.err
}

func TestTokenResolver_ResolveToken(t *testing.T) {
	tests := []struct {
		name    string
		users   []*models.User
		err     error
		want    string
		wantErr bool
	}{
		{
			name: "returns first user with token",
			users: []*models.User{
				{ID: 1, AccessToken: ""},
				{ID: 2, AccessToken: "token-abc"},
				{ID: 3, AccessToken: "token-xyz"},
			},
			want: "token-abc",
		},
		{
			name: "no users with tokens",
			users: []*models.User{
				{ID: 1, AccessToken: ""},
			},
			wantErr: true,
		},
		{
			name:    "no users at all",
			users:   nil,
			wantErr: true,
		},
		{
			name:    "repository error",
			err:     fmt.Errorf("db error"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewTokenResolver(&mockProjectUserGetter{users: tt.users, err: tt.err})
			got, err := resolver.ResolveToken(context.Background(), 42)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
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
