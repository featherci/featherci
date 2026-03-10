package services

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/featherci/featherci/internal/crypto"
	"github.com/featherci/featherci/internal/models"
)

// memSecretRepo is an in-memory SecretRepository for testing.
type memSecretRepo struct {
	secrets []*models.Secret
	nextID  int64
}

func (m *memSecretRepo) Create(_ context.Context, s *models.Secret) error {
	m.nextID++
	s.ID = m.nextID
	m.secrets = append(m.secrets, s)
	return nil
}

func (m *memSecretRepo) GetByName(_ context.Context, projectID int64, name string) (*models.Secret, error) {
	for _, s := range m.secrets {
		if s.ProjectID == projectID && s.Name == name {
			return s, nil
		}
	}
	return nil, models.ErrNotFound
}

func (m *memSecretRepo) ListByProject(_ context.Context, projectID int64) ([]*models.Secret, error) {
	var result []*models.Secret
	for _, s := range m.secrets {
		if s.ProjectID == projectID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *memSecretRepo) Update(_ context.Context, secret *models.Secret) error {
	for i, s := range m.secrets {
		if s.ID == secret.ID {
			m.secrets[i] = secret
			return nil
		}
	}
	return models.ErrNotFound
}

func (m *memSecretRepo) Delete(_ context.Context, projectID int64, name string) error {
	for i, s := range m.secrets {
		if s.ProjectID == projectID && s.Name == name {
			m.secrets = append(m.secrets[:i], m.secrets[i+1:]...)
			return nil
		}
	}
	return models.ErrNotFound
}

func newTestService(t *testing.T) *SecretService {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatal(err)
	}
	return NewSecretService(&memSecretRepo{}, enc)
}

func TestCreateAndList(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if err := svc.CreateSecret(ctx, 1, "API_KEY", "abc123", 1); err != nil {
		t.Fatal(err)
	}

	secrets, err := svc.ListSecrets(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}
	if secrets[0].Name != "API_KEY" {
		t.Fatalf("expected API_KEY, got %s", secrets[0].Name)
	}
}

func TestCreateDuplicate(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if err := svc.CreateSecret(ctx, 1, "TOKEN", "val", 1); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateSecret(ctx, 1, "TOKEN", "val2", 1); err == nil {
		t.Fatal("expected error for duplicate")
	}
}

func TestInvalidName(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	cases := []string{"", "1starts-with-num", "has space", "has-dash", "a@b"}
	for _, name := range cases {
		if err := svc.CreateSecret(ctx, 1, name, "val", 1); err == nil {
			t.Errorf("expected error for name %q", name)
		}
	}
}

func TestGetDecryptedSecrets(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.CreateSecret(ctx, 1, "KEY_A", "value_a", 1)
	svc.CreateSecret(ctx, 1, "KEY_B", "value_b", 1)

	m, err := svc.GetDecryptedSecrets(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if m["KEY_A"] != "value_a" || m["KEY_B"] != "value_b" {
		t.Fatalf("unexpected decrypted values: %v", m)
	}
}

func TestUpdateSecret(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.CreateSecret(ctx, 1, "MY_SECRET", "old", 1)

	if err := svc.UpdateSecret(ctx, 1, "MY_SECRET", "new"); err != nil {
		t.Fatal(err)
	}

	m, err := svc.GetDecryptedSecrets(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if m["MY_SECRET"] != "new" {
		t.Fatalf("expected 'new', got %q", m["MY_SECRET"])
	}
}

func TestDeleteSecret(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.CreateSecret(ctx, 1, "DEL_ME", "val", 1)

	if err := svc.DeleteSecret(ctx, 1, "DEL_ME"); err != nil {
		t.Fatal(err)
	}

	secrets, err := svc.ListSecrets(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 0 {
		t.Fatalf("expected 0 secrets, got %d", len(secrets))
	}
}
