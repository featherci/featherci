package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/featherci/featherci/internal/middleware"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/web/static"
)

// PageData holds common data passed to all page templates.
type PageData struct {
	User      *models.User
	Providers []string
	DevMode   bool
}

// LoginPageData holds data for the login page.
type LoginPageData struct {
	PageData
}

// DashboardPageData holds data for the dashboard page.
type DashboardPageData struct {
	PageData
	Stats        DashboardStats
	RecentBuilds []BuildSummary
}

// DashboardStats holds statistics for the dashboard.
type DashboardStats struct {
	ProjectCount int
	SuccessCount int
	FailureCount int
	RunningCount int
}

// BuildSummary holds a summary of a build for display.
type BuildSummary struct {
	ID          int64
	ProjectID   int64
	ProjectName string
	Status      string
	Branch      string
	CommitSHA   string
	Duration    time.Duration
	StartedAt   time.Time
}

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

	// Projects
	mux.Handle("GET /projects", s.authMiddleware.RequireAuth(http.HandlerFunc(s.projectHandler.List)))
	mux.Handle("GET /projects/new", s.authMiddleware.RequireAuth(http.HandlerFunc(s.projectHandler.New)))
	mux.Handle("POST /projects", s.authMiddleware.RequireAuth(http.HandlerFunc(s.projectHandler.Create)))
	mux.Handle("GET /projects/{namespace}/{name}", s.authMiddleware.RequireAuth(http.HandlerFunc(s.projectHandler.Show)))
	mux.Handle("GET /projects/{namespace}/{name}/settings", s.authMiddleware.RequireAuth(http.HandlerFunc(s.projectHandler.Settings)))
	mux.Handle("POST /projects/{namespace}/{name}/settings", s.authMiddleware.RequireAuth(http.HandlerFunc(s.projectHandler.Update)))
	mux.Handle("POST /projects/{namespace}/{name}/delete", s.authMiddleware.RequireAuth(http.HandlerFunc(s.projectHandler.Delete)))

	// Build actions
	mux.Handle("POST /projects/{namespace}/{name}/builds/{number}/cancel",
		s.authMiddleware.RequireAuth(http.HandlerFunc(s.buildHandler.Cancel)))

	// API routes for workers
	mux.HandleFunc("GET /api/worker/jobs", s.handleNotImplemented)
	mux.HandleFunc("POST /api/worker/jobs/{id}/status", s.handleNotImplemented)
	mux.HandleFunc("POST /api/worker/heartbeat", s.handleNotImplemented)

	// Webhooks (no auth, validated by signature)
	mux.HandleFunc("POST /webhooks/github", s.webhookHandler.HandleGitHub)
	mux.HandleFunc("POST /webhooks/gitlab", s.webhookHandler.HandleGitLab)
	mux.HandleFunc("POST /webhooks/gitea", s.webhookHandler.HandleGitea)

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

	if user != nil {
		// Get project count for user
		projects, err := s.projectUsers.GetProjectsForUserWithStatus(r.Context(), user.ID)
		if err != nil {
			s.logger.Error("failed to get projects for dashboard", "error", err)
			projects = nil
		}

		// Authenticated user - show dashboard
		data := DashboardPageData{
			PageData: PageData{
				User:      user,
				Providers: s.providers.Available(),
				DevMode:   s.config.DevMode,
			},
			Stats: DashboardStats{
				ProjectCount: len(projects),
				SuccessCount: 0, // TODO: Get from builds table
				FailureCount: 0,
				RunningCount: 0,
			},
			RecentBuilds: nil, // TODO: Get from database
		}

		if err := s.templates.Render(w, "pages/dashboard.html", data); err != nil {
			s.logger.Error("failed to render dashboard", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	} else {
		// Anonymous user - show login page
		data := LoginPageData{
			PageData: PageData{
				User:      nil,
				Providers: s.providers.Available(),
				DevMode:   s.config.DevMode,
			},
		}

		if err := s.templates.Render(w, "pages/login.html", data); err != nil {
			s.logger.Error("failed to render login page", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}

// handleNotImplemented returns a 501 Not Implemented response.
func (s *Server) handleNotImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"})
}
