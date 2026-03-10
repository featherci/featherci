package services

import (
	"context"
	"fmt"
	"regexp"

	"github.com/featherci/featherci/internal/crypto"
	"github.com/featherci/featherci/internal/models"
)

var secretNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const maxSecretNameLength = 128

// SecretService provides business logic for managing encrypted secrets.
type SecretService struct {
	secrets   models.SecretRepository
	encryptor *crypto.Encryptor
}

// NewSecretService creates a new SecretService.
func NewSecretService(secrets models.SecretRepository, encryptor *crypto.Encryptor) *SecretService {
	return &SecretService{
		secrets:   secrets,
		encryptor: encryptor,
	}
}

// CreateSecret validates, encrypts, and stores a new secret.
func (s *SecretService) CreateSecret(ctx context.Context, projectID int64, name, value string, userID int64) error {
	if err := validateSecretName(name); err != nil {
		return err
	}

	// Check for duplicate
	existing, err := s.secrets.GetByName(ctx, projectID, name)
	if err != nil && err != models.ErrNotFound {
		return fmt.Errorf("checking existing secret: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("secret %q already exists", name)
	}

	encrypted, err := s.encryptor.Encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("encrypting secret: %w", err)
	}

	secret := &models.Secret{
		ProjectID:      projectID,
		Name:           name,
		EncryptedValue: encrypted,
		CreatedBy:      userID,
	}

	return s.secrets.Create(ctx, secret)
}

// UpdateSecret re-encrypts and updates an existing secret.
func (s *SecretService) UpdateSecret(ctx context.Context, projectID int64, name, value string) error {
	existing, err := s.secrets.GetByName(ctx, projectID, name)
	if err != nil {
		return fmt.Errorf("getting secret: %w", err)
	}

	encrypted, err := s.encryptor.Encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("encrypting secret: %w", err)
	}

	existing.EncryptedValue = encrypted
	return s.secrets.Update(ctx, existing)
}

// DeleteSecret removes a secret.
func (s *SecretService) DeleteSecret(ctx context.Context, projectID int64, name string) error {
	return s.secrets.Delete(ctx, projectID, name)
}

// ListSecrets returns secret metadata (no decrypted values).
func (s *SecretService) ListSecrets(ctx context.Context, projectID int64) ([]*models.Secret, error) {
	return s.secrets.ListByProject(ctx, projectID)
}

// GetDecryptedSecrets returns all secrets for a project as a name→value map.
func (s *SecretService) GetDecryptedSecrets(ctx context.Context, projectID int64) (map[string]string, error) {
	secrets, err := s.secrets.ListByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	result := make(map[string]string, len(secrets))
	for _, sec := range secrets {
		plaintext, err := s.encryptor.Decrypt(sec.EncryptedValue)
		if err != nil {
			return nil, fmt.Errorf("decrypting secret %q: %w", sec.Name, err)
		}
		result[sec.Name] = string(plaintext)
	}

	return result, nil
}

func validateSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("secret name is required")
	}
	if len(name) > maxSecretNameLength {
		return fmt.Errorf("secret name must be at most %d characters", maxSecretNameLength)
	}
	if !secretNamePattern.MatchString(name) {
		return fmt.Errorf("secret name must match %s", secretNamePattern.String())
	}
	return nil
}
