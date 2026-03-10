---
model: sonnet
---

# Step 20: Encrypted Secrets Management

## Objective
Implement secure storage and injection of project secrets using AES-256-GCM encryption.

## Tasks

### 20.1 Create Crypto Package
```go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "errors"
    "io"
)

type Encryptor struct {
    key []byte // 32 bytes for AES-256
}

func NewEncryptor(key []byte) (*Encryptor, error) {
    if len(key) != 32 {
        return nil, errors.New("key must be 32 bytes for AES-256")
    }
    return &Encryptor{key: key}, nil
}

func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
    block, err := aes.NewCipher(e.key)
    if err != nil {
        return nil, err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    
    // Create random nonce
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, err
    }
    
    // Encrypt and prepend nonce
    ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
    return ciphertext, nil
}

func (e *Encryptor) Decrypt(ciphertext []byte) ([]byte, error) {
    block, err := aes.NewCipher(e.key)
    if err != nil {
        return nil, err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    
    nonceSize := gcm.NonceSize()
    if len(ciphertext) < nonceSize {
        return nil, errors.New("ciphertext too short")
    }
    
    nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
    return gcm.Open(nil, nonce, ciphertext, nil)
}

// For storing in database as base64
func (e *Encryptor) EncryptString(plaintext string) (string, error) {
    encrypted, err := e.Encrypt([]byte(plaintext))
    if err != nil {
        return "", err
    }
    return base64.StdEncoding.EncodeToString(encrypted), nil
}

func (e *Encryptor) DecryptString(ciphertext string) (string, error) {
    data, err := base64.StdEncoding.DecodeString(ciphertext)
    if err != nil {
        return "", err
    }
    decrypted, err := e.Decrypt(data)
    if err != nil {
        return "", err
    }
    return string(decrypted), nil
}
```

### 20.2 Create Secret Model
```go
type Secret struct {
    ID             int64
    ProjectID      int64
    Name           string
    EncryptedValue []byte
    CreatedBy      int64
    CreatedAt      time.Time
    UpdatedAt      time.Time
    
    // Loaded via join
    CreatedByUser *User
}

type SecretRepository interface {
    Create(ctx context.Context, secret *Secret) error
    GetByID(ctx context.Context, id int64) (*Secret, error)
    GetByName(ctx context.Context, projectID int64, name string) (*Secret, error)
    ListByProject(ctx context.Context, projectID int64) ([]*Secret, error)
    Update(ctx context.Context, secret *Secret) error
    Delete(ctx context.Context, id int64) error
}
```

### 20.3 Create Secret Service
```go
type SecretService struct {
    secrets   SecretRepository
    encryptor *crypto.Encryptor
}

func NewSecretService(secrets SecretRepository, key []byte) (*SecretService, error) {
    enc, err := crypto.NewEncryptor(key)
    if err != nil {
        return nil, err
    }
    return &SecretService{
        secrets:   secrets,
        encryptor: enc,
    }, nil
}

func (s *SecretService) CreateSecret(ctx context.Context, projectID int64, name, value string, userID int64) error {
    // Validate secret name (alphanumeric, underscore only)
    if !isValidSecretName(name) {
        return errors.New("invalid secret name: must be alphanumeric with underscores")
    }
    
    // Check for duplicate
    existing, _ := s.secrets.GetByName(ctx, projectID, name)
    if existing != nil {
        return errors.New("secret already exists")
    }
    
    // Encrypt value
    encrypted, err := s.encryptor.Encrypt([]byte(value))
    if err != nil {
        return fmt.Errorf("encryption failed: %w", err)
    }
    
    secret := &Secret{
        ProjectID:      projectID,
        Name:           name,
        EncryptedValue: encrypted,
        CreatedBy:      userID,
    }
    
    return s.secrets.Create(ctx, secret)
}

func (s *SecretService) UpdateSecret(ctx context.Context, id int64, value string) error {
    encrypted, err := s.encryptor.Encrypt([]byte(value))
    if err != nil {
        return err
    }
    
    secret, err := s.secrets.GetByID(ctx, id)
    if err != nil {
        return err
    }
    
    secret.EncryptedValue = encrypted
    return s.secrets.Update(ctx, secret)
}

func (s *SecretService) GetDecryptedSecrets(ctx context.Context, projectID int64) (map[string]string, error) {
    secrets, err := s.secrets.ListByProject(ctx, projectID)
    if err != nil {
        return nil, err
    }
    
    result := make(map[string]string)
    for _, secret := range secrets {
        decrypted, err := s.encryptor.Decrypt(secret.EncryptedValue)
        if err != nil {
            return nil, fmt.Errorf("failed to decrypt secret %s: %w", secret.Name, err)
        }
        result[secret.Name] = string(decrypted)
    }
    
    return result, nil
}

func isValidSecretName(name string) bool {
    if len(name) == 0 || len(name) > 128 {
        return false
    }
    for _, c := range name {
        if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || 
             (c >= '0' && c <= '9') || c == '_') {
            return false
        }
    }
    return true
}
```

### 20.4 Create Secrets HTTP Handlers
```go
type SecretsHandler struct {
    secrets      *SecretService
    projects     ProjectRepository
    projectUsers ProjectUserRepository
}

func (h *SecretsHandler) List(w http.ResponseWriter, r *http.Request) {
    project := getProjectFromContext(r)
    user := getUserFromContext(r)
    
    // Check permission
    canManage, _ := h.projectUsers.CanUserManage(r.Context(), project.ID, user.ID)
    if !canManage {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }
    
    secrets, _ := h.secrets.ListByProject(r.Context(), project.ID)
    
    render(w, "secrets/list.html", map[string]any{
        "Project": project,
        "Secrets": secrets, // Values are NOT included
    })
}

func (h *SecretsHandler) Create(w http.ResponseWriter, r *http.Request) {
    project := getProjectFromContext(r)
    user := getUserFromContext(r)
    
    name := r.FormValue("name")
    value := r.FormValue("value")
    
    err := h.secrets.CreateSecret(r.Context(), project.ID, name, value, user.ID)
    if err != nil {
        // Handle error
        return
    }
    
    http.Redirect(w, r, fmt.Sprintf("/projects/%s/secrets", project.FullName), http.StatusSeeOther)
}

func (h *SecretsHandler) Delete(w http.ResponseWriter, r *http.Request) {
    secretName := r.PathValue("name")
    project := getProjectFromContext(r)
    
    secret, _ := h.secrets.GetByName(r.Context(), project.ID, secretName)
    if secret == nil {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }
    
    h.secrets.Delete(r.Context(), secret.ID)
    
    http.Redirect(w, r, fmt.Sprintf("/projects/%s/secrets", project.FullName), http.StatusSeeOther)
}
```

### 20.5 Create Secrets UI
`web/templates/pages/secrets/list.html`:
```html
{{template "base" .}}

{{define "title"}}Secrets - {{.Project.FullName}} - FeatherCI{{end}}

{{define "content"}}
<div class="space-y-6">
    <h1 class="text-2xl font-bold text-gray-900">Secrets</h1>
    <p class="text-gray-600">
        Secrets are encrypted and injected as environment variables during builds.
    </p>
    
    <!-- Add secret form -->
    <div class="card p-6">
        <h2 class="text-lg font-medium mb-4">Add Secret</h2>
        <form action="/projects/{{.Project.Namespace}}/{{.Project.Name}}/secrets" method="POST" class="space-y-4">
            <div>
                <label for="name" class="block text-sm font-medium text-gray-700">Name</label>
                <input type="text" name="name" id="name" required
                       pattern="[A-Za-z_][A-Za-z0-9_]*"
                       placeholder="MY_SECRET_KEY"
                       class="input mt-1">
                <p class="text-xs text-gray-500 mt-1">
                    Alphanumeric characters and underscores only
                </p>
            </div>
            <div>
                <label for="value" class="block text-sm font-medium text-gray-700">Value</label>
                <input type="password" name="value" id="value" required
                       class="input mt-1">
            </div>
            <button type="submit" class="btn btn-primary">Add Secret</button>
        </form>
    </div>
    
    <!-- Existing secrets -->
    <div class="card">
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">By</th>
                    <th class="px-6 py-3"></th>
                </tr>
            </thead>
            <tbody class="divide-y divide-gray-200">
                {{range .Secrets}}
                <tr>
                    <td class="px-6 py-4 font-mono text-sm">{{.Name}}</td>
                    <td class="px-6 py-4 text-sm text-gray-500">{{.CreatedAt | timeAgo}}</td>
                    <td class="px-6 py-4 text-sm text-gray-500">{{.CreatedByUser.Username}}</td>
                    <td class="px-6 py-4 text-right">
                        <form action="/projects/{{$.Project.Namespace}}/{{$.Project.Name}}/secrets/{{.Name}}" 
                              method="POST" 
                              onsubmit="return confirm('Delete this secret?')">
                            <input type="hidden" name="_method" value="DELETE">
                            <button type="submit" class="text-red-600 hover:text-red-800 text-sm">
                                Delete
                            </button>
                        </form>
                    </td>
                </tr>
                {{else}}
                <tr>
                    <td colspan="4" class="px-6 py-8 text-center text-gray-500">
                        No secrets configured
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
</div>
{{end}}
```

### 20.6 Inject Secrets into Build Steps
In the step runner:
```go
func (r *StepRunner) RunStep(ctx context.Context, step *BuildStep, build *Build, project *Project) error {
    // Get decrypted secrets
    secrets, err := r.secrets.GetDecryptedSecrets(ctx, project.ID)
    if err != nil {
        return fmt.Errorf("failed to get secrets: %w", err)
    }
    
    // Merge with step env (step env takes precedence)
    env := make(map[string]string)
    for k, v := range secrets {
        env[k] = v
    }
    for k, v := range step.Env {
        env[k] = v
    }
    
    // ... continue with execution
}
```

### 20.7 Add Tests
- Test encryption/decryption
- Test secret creation with validation
- Test duplicate detection
- Test secret injection into env

## Deliverables
- [ ] `internal/crypto/encryptor.go` - AES-256-GCM encryption
- [ ] `internal/models/secret.go` - Secret model
- [ ] `internal/services/secrets.go` - Secret service
- [ ] `internal/handlers/secrets.go` - HTTP handlers
- [ ] `web/templates/pages/secrets/list.html` - Secrets UI
- [ ] Secrets encrypted at rest
- [ ] Secrets injected into builds

## Dependencies
- Step 09: Project management
- Step 14: Docker executor (for injection)

## Estimated Effort
Medium - Security-critical feature
