---
model: sonnet
---

# Step 05: User Management and Sessions

## Objective
Implement user creation, session management, and authorization logic.

## Tasks

### 5.1 Create User Model
```go
type User struct {
    ID           int64
    Provider     string
    ProviderID   string
    Username     string
    Email        string
    AvatarURL    string
    AccessToken  string    // Encrypted in DB
    RefreshToken string    // Encrypted in DB
    IsAdmin      bool
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

type UserRepository interface {
    Create(ctx context.Context, user *User) error
    GetByID(ctx context.Context, id int64) (*User, error)
    GetByProviderID(ctx context.Context, provider, providerID string) (*User, error)
    Update(ctx context.Context, user *User) error
    UpdateTokens(ctx context.Context, id int64, access, refresh string) error
    List(ctx context.Context) ([]*User, error)
    Delete(ctx context.Context, id int64) error
}
```

### 5.2 Create Session Management
```go
type Session struct {
    ID        string    // Random token
    UserID    int64
    ExpiresAt time.Time
    CreatedAt time.Time
}

type SessionStore interface {
    Create(ctx context.Context, userID int64) (*Session, error)
    Get(ctx context.Context, sessionID string) (*Session, error)
    Delete(ctx context.Context, sessionID string) error
    DeleteAllForUser(ctx context.Context, userID int64) error
    Cleanup(ctx context.Context) error // Remove expired sessions
}
```

Use SQLite for session storage (simple, persistent):
```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
```

### 5.3 Create Session Cookie Handling
```go
const (
    SessionCookieName = "featherci_session"
    SessionDuration   = 30 * 24 * time.Hour // 30 days
)

func SetSessionCookie(w http.ResponseWriter, session *Session, secure bool)
func GetSessionFromRequest(r *http.Request) (string, error)
func ClearSessionCookie(w http.ResponseWriter)
```

### 5.4 Create Auth Middleware
```go
type AuthMiddleware struct {
    sessions SessionStore
    users    UserRepository
}

func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler
func (m *AuthMiddleware) OptionalAuth(next http.Handler) http.Handler

// Context helpers
type contextKey string
const UserContextKey contextKey = "user"

func UserFromContext(ctx context.Context) *User
func SetUserInContext(ctx context.Context, user *User) context.Context
```

### 5.5 Create Admin Check Logic
```go
func (u *User) CanManageProject(project *Project) bool
func (u *User) IsAdminConfigured(admins []string) bool
func IsUsernameAdmin(username, provider string, admins []string) bool
```

### 5.6 Implement OAuth Callback Handler
```go
type AuthHandler struct {
    providers *auth.Registry
    users     UserRepository
    sessions  SessionStore
    config    *config.Config
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request)      // GET /auth/:provider
func (h *AuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request)   // GET /auth/:provider/callback
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request)     // POST /auth/logout
```

### 5.7 State Token for CSRF Protection
```go
func GenerateStateToken() (string, error)
func ValidateStateToken(expected, actual string) bool
```

Store state in short-lived cookie or in-memory with expiration.

### 5.8 Add Tests
- Test user CRUD operations
- Test session creation/validation
- Test cookie handling
- Test middleware auth checks
- Test admin detection

## Deliverables
- [ ] `internal/models/user.go` - User model and repository
- [ ] `internal/models/session.go` - Session management
- [ ] `internal/handlers/auth.go` - Auth HTTP handlers
- [ ] `internal/middleware/auth.go` - Auth middleware
- [ ] Tests for all components

## Dependencies
- Step 03: Database (users table)
- Step 04: OAuth providers

## Estimated Effort
Medium - Core authentication flow
