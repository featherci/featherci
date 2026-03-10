package server

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/featherci/featherci/internal/middleware"
	"github.com/featherci/featherci/web/static"
)

// setupRoutes configures all HTTP routes.
func (s *Server) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Static files (CSS, JS, images) - served from embedded filesystem
	staticFS, err := fs.Sub(static.Files, ".")
	if err != nil {
		s.logger.Error("failed to create static file sub-filesystem", "error", err)
	} else {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}

	// Health check endpoints (no auth required)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)

	// Auth routes
	mux.HandleFunc("GET /auth/{provider}", s.authHandler.HandleLogin)
	mux.HandleFunc("GET /auth/{provider}/callback", s.authHandler.HandleCallback)
	mux.HandleFunc("POST /auth/logout", s.authHandler.HandleLogout)

	// Dev login (only available in dev mode)
	if s.config.DevMode {
		mux.HandleFunc("GET /auth/dev", s.authHandler.HandleDevLogin)
	}

	// Dashboard (home page) - optional auth
	mux.Handle("GET /", s.authMiddleware.OptionalAuth(http.HandlerFunc(s.handleDashboard)))

	// Placeholder routes - these will be implemented in future steps
	// Projects
	mux.Handle("GET /projects", s.authMiddleware.RequireAuth(http.HandlerFunc(s.handleNotImplemented)))
	mux.Handle("GET /projects/new", s.authMiddleware.RequireAuth(http.HandlerFunc(s.handleNotImplemented)))
	mux.Handle("POST /projects", s.authMiddleware.RequireAuth(http.HandlerFunc(s.handleNotImplemented)))

	// API routes for workers
	mux.HandleFunc("GET /api/worker/jobs", s.handleNotImplemented)
	mux.HandleFunc("POST /api/worker/jobs/{id}/status", s.handleNotImplemented)
	mux.HandleFunc("POST /api/worker/heartbeat", s.handleNotImplemented)

	// Webhooks (no auth, validated by signature)
	mux.HandleFunc("POST /webhooks/github", s.handleNotImplemented)
	mux.HandleFunc("POST /webhooks/gitlab", s.handleNotImplemented)
	mux.HandleFunc("POST /webhooks/gitea", s.handleNotImplemented)

	return mux
}

// handleHealth returns a simple health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleReady checks if the server is ready to accept traffic.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// Check database connection
	if err := s.db.Ping(); err != nil {
		s.logger.Error("readiness check failed", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready", "error": "database unavailable"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// handleDashboard renders the home page.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Get user from context (may be nil if not authenticated)
	user := middleware.UserFromContext(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Simple HTML response for now - will be replaced with templates in Step 08
	if user != nil {
		w.Write([]byte(dashboardHTML(user.Username, user.AvatarURL, user.IsAdmin, s.config.DevMode)))
	} else {
		w.Write([]byte(loginPageHTML(s.providers.Available(), s.config.DevMode)))
	}
}

// handleNotImplemented returns a 501 Not Implemented response.
func (s *Server) handleNotImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"})
}

// loginPageHTML generates a simple login page.
func loginPageHTML(providers []string, devMode bool) string {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FeatherCI - Login</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f5f5; min-height: 100vh; display: flex; align-items: center; justify-content: center; }
        .container { background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); max-width: 400px; width: 100%; }
        h1 { font-size: 1.5rem; margin-bottom: 0.5rem; color: #333; }
        p { color: #666; margin-bottom: 1.5rem; }
        .btn { display: block; width: 100%; padding: 0.75rem; margin-bottom: 0.5rem; border: none; border-radius: 4px; font-size: 1rem; cursor: pointer; text-decoration: none; text-align: center; }
        .btn-github { background: #24292e; color: white; }
        .btn-gitlab { background: #fc6d26; color: white; }
        .btn-gitea { background: #609926; color: white; }
        .btn-dev { background: #6c757d; color: white; }
        .btn:hover { opacity: 0.9; }
        .divider { text-align: center; margin: 1rem 0; color: #999; }
    </style>
</head>
<body>
    <div class="container">
        <h1>FeatherCI</h1>
        <p>Sign in to continue</p>`

	for _, provider := range providers {
		switch provider {
		case "github":
			html += `<a href="/auth/github" class="btn btn-github">Sign in with GitHub</a>`
		case "gitlab":
			html += `<a href="/auth/gitlab" class="btn btn-gitlab">Sign in with GitLab</a>`
		case "gitea":
			html += `<a href="/auth/gitea" class="btn btn-gitea">Sign in with Gitea</a>`
		}
	}

	if devMode {
		if len(providers) > 0 {
			html += `<div class="divider">or</div>`
		}
		html += `<a href="/auth/dev" class="btn btn-dev">Dev Login (Admin)</a>`
	}

	html += `
    </div>
</body>
</html>`
	return html
}

// dashboardHTML generates a simple dashboard page.
func dashboardHTML(username, avatarURL string, isAdmin, devMode bool) string {
	adminBadge := ""
	if isAdmin {
		adminBadge = `<span style="background:#28a745;color:white;padding:0.25rem 0.5rem;border-radius:4px;font-size:0.75rem;margin-left:0.5rem;">Admin</span>`
	}

	avatar := ""
	if avatarURL != "" {
		avatar = `<img src="` + avatarURL + `" alt="Avatar" style="width:48px;height:48px;border-radius:50%;margin-right:1rem;">`
	}

	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FeatherCI - Dashboard</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f5f5; min-height: 100vh; }
        header { background: white; border-bottom: 1px solid #e1e4e8; padding: 1rem 2rem; display: flex; justify-content: space-between; align-items: center; }
        .logo { font-weight: bold; font-size: 1.25rem; }
        .user { display: flex; align-items: center; }
        .username { font-weight: 500; }
        .btn { padding: 0.5rem 1rem; border: none; border-radius: 4px; cursor: pointer; text-decoration: none; }
        .btn-logout { background: #dc3545; color: white; margin-left: 1rem; }
        .container { max-width: 1200px; margin: 2rem auto; padding: 0 2rem; }
        .card { background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
        h2 { margin-bottom: 1rem; }
        .coming-soon { color: #666; }
    </style>
</head>
<body>
    <header>
        <div class="logo">FeatherCI</div>
        <div class="user">
            ` + avatar + `
            <span class="username">` + username + `</span>` + adminBadge + `
            <form action="/auth/logout" method="POST" style="display:inline;">
                <button type="submit" class="btn btn-logout">Logout</button>
            </form>
        </div>
    </header>
    <div class="container">
        <div class="card">
            <h2>Welcome to FeatherCI!</h2>
            <p class="coming-soon">Project management and build UI coming soon...</p>
        </div>
    </div>
</body>
</html>`
}
