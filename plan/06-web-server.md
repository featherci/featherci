---
model: sonnet
---

# Step 06: Web Server and Routing

## Objective
Set up the HTTP server with routing, static file serving, and template rendering.

## Tasks

### 6.1 Set Up HTTP Server
```go
type Server struct {
    config   *config.Config
    db       *sqlx.DB
    router   *http.ServeMux
    handlers *Handlers
}

func NewServer(cfg *config.Config, db *sqlx.DB) *Server
func (s *Server) Start(ctx context.Context) error
func (s *Server) Shutdown(ctx context.Context) error
```

### 6.2 Create Router with Go 1.22 Patterns
Using the new Go 1.22 `http.ServeMux` with method patterns:
```go
mux := http.NewServeMux()

// Static files
mux.Handle("GET /static/", http.StripPrefix("/static/", staticFS))

// Auth routes
mux.HandleFunc("GET /auth/{provider}", handlers.Login)
mux.HandleFunc("GET /auth/{provider}/callback", handlers.Callback)
mux.HandleFunc("POST /auth/logout", handlers.Logout)

// Dashboard
mux.HandleFunc("GET /", handlers.Dashboard)

// Projects
mux.HandleFunc("GET /projects", handlers.ListProjects)
mux.HandleFunc("GET /projects/new", handlers.NewProject)
mux.HandleFunc("POST /projects", handlers.CreateProject)
mux.HandleFunc("GET /projects/{owner}/{repo}", handlers.ShowProject)
mux.HandleFunc("GET /projects/{owner}/{repo}/settings", handlers.ProjectSettings)
mux.HandleFunc("POST /projects/{owner}/{repo}/settings", handlers.UpdateProject)

// Builds
mux.HandleFunc("GET /projects/{owner}/{repo}/builds", handlers.ListBuilds)
mux.HandleFunc("GET /projects/{owner}/{repo}/builds/{number}", handlers.ShowBuild)
mux.HandleFunc("POST /projects/{owner}/{repo}/builds/{number}/cancel", handlers.CancelBuild)
mux.HandleFunc("POST /projects/{owner}/{repo}/builds/{number}/steps/{step}/approve", handlers.ApproveStep)

// Secrets
mux.HandleFunc("GET /projects/{owner}/{repo}/secrets", handlers.ListSecrets)
mux.HandleFunc("POST /projects/{owner}/{repo}/secrets", handlers.CreateSecret)
mux.HandleFunc("DELETE /projects/{owner}/{repo}/secrets/{name}", handlers.DeleteSecret)

// Webhooks (no auth)
mux.HandleFunc("POST /webhooks/github", handlers.GitHubWebhook)
mux.HandleFunc("POST /webhooks/gitlab", handlers.GitLabWebhook)
mux.HandleFunc("POST /webhooks/gitea", handlers.GiteaWebhook)

// Worker API (token auth)
mux.HandleFunc("GET /api/worker/jobs", handlers.GetJob)
mux.HandleFunc("POST /api/worker/jobs/{id}/status", handlers.UpdateJobStatus)
mux.HandleFunc("POST /api/worker/heartbeat", handlers.WorkerHeartbeat)

// SSE for live logs
mux.HandleFunc("GET /projects/{owner}/{repo}/builds/{number}/steps/{step}/logs", handlers.StreamLogs)

// Admin routes
mux.HandleFunc("GET /admin/users", handlers.AdminListUsers)
mux.HandleFunc("POST /admin/users", handlers.AdminAddUser)
mux.HandleFunc("DELETE /admin/users/{id}", handlers.AdminRemoveUser)
```

### 6.3 Create Middleware Stack
```go
func ChainMiddleware(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler

// Middleware to apply:
// - Logging
// - Recovery (panic handling)
// - Request ID
// - CORS (if needed)
// - Auth (per-route)
```

### 6.4 Create Request Logging Middleware
```go
type LoggingMiddleware struct {
    logger *slog.Logger
}

func (m *LoggingMiddleware) Handler(next http.Handler) http.Handler
```

Log:
- Request method, path
- Response status code
- Duration
- Request ID

### 6.5 Create Recovery Middleware
```go
func RecoveryMiddleware(next http.Handler) http.Handler
```

Catch panics, log stack trace, return 500.

### 6.6 Add Health Check Endpoint
```go
mux.HandleFunc("GET /health", handlers.Health)
mux.HandleFunc("GET /ready", handlers.Ready)
```

### 6.7 Wire Up in Main
```go
func main() {
    cfg, err := config.Load()
    // ...
    db, err := database.Open(cfg.DatabasePath)
    // ...
    srv := server.NewServer(cfg, db)
    
    // Graceful shutdown
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()
    
    if err := srv.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## Deliverables
- [ ] `internal/server/server.go` - HTTP server setup
- [ ] `internal/server/routes.go` - Route definitions
- [ ] `internal/middleware/logging.go` - Request logging
- [ ] `internal/middleware/recovery.go` - Panic recovery
- [ ] `cmd/featherci/main.go` - Updated with server startup
- [ ] Health check endpoints working

## Dependencies
- Step 02: Configuration
- Step 03: Database
- Step 05: User management (for auth middleware)

## Estimated Effort
Medium - Server infrastructure
