// Package server provides the HTTP server for FeatherCI.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/featherci/featherci/internal/auth"
	"github.com/featherci/featherci/internal/config"
	"github.com/featherci/featherci/internal/crypto"
	"github.com/featherci/featherci/internal/database"
	"github.com/featherci/featherci/internal/gitclient"
	"github.com/featherci/featherci/internal/handlers"
	"github.com/featherci/featherci/internal/middleware"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/services"
	"github.com/featherci/featherci/internal/templates"
	"github.com/featherci/featherci/internal/worker"
	"github.com/featherci/featherci/internal/workflow"
)

// Server represents the FeatherCI HTTP server.
type Server struct {
	config         *config.Config
	db             *database.DB
	httpServer     *http.Server
	logger         *slog.Logger
	providers      *auth.Registry
	users          models.UserRepository
	sessions       models.SessionStore
	projects       models.ProjectRepository
	projectUsers   models.ProjectUserRepository
	builds         models.BuildRepository
	templates      *templates.Engine
	authHandler    *handlers.AuthHandler
	projectHandler *handlers.ProjectHandler
	webhookHandler *handlers.WebhookHandler
	buildHandler   *handlers.BuildHandler
	secretHandler  *handlers.SecretHandler
	authMiddleware *middleware.AuthMiddleware
}

// New creates a new Server instance.
func New(cfg *config.Config, db *database.DB, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Initialize template engine
	tmpl, err := templates.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize templates: %w", err)
	}

	// Initialize repositories
	users := models.NewUserRepository(db.DB)
	sessions := models.NewSessionStore(db.DB)
	projects := models.NewProjectRepository(db.DB)
	projectUsers := models.NewProjectUserRepository(db.DB)
	builds := models.NewBuildRepository(db.DB)
	steps := models.NewBuildStepRepository(db.DB)

	// Initialize OAuth providers
	providers := auth.NewRegistry(cfg)

	// Initialize auth middleware
	authMiddleware := middleware.NewAuthMiddleware(sessions, users)

	// Initialize build pipeline dependencies
	fileFetcher := gitclient.NewFileContentFetcher(cfg.GitLabURL, cfg.GiteaURL)
	parser := workflow.NewParser()
	tokenSource := worker.NewProjectTokenSource(projectUsers)
	buildCreator := services.NewBuildCreator(builds, steps)

	// Initialize secrets
	encryptor, err := crypto.NewEncryptor(cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}
	secrets := models.NewSecretRepository(db.DB)
	secretService := services.NewSecretService(secrets, encryptor)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(providers, users, sessions, cfg)
	projectHandler := handlers.NewProjectHandler(projects, projectUsers, users, builds, secrets, providers, tmpl, logger)
	webhookHandler := handlers.NewWebhookHandler(projects, logger, buildCreator, fileFetcher, tokenSource, parser)
	buildHandler := handlers.NewBuildHandler(projects, builds, steps, tmpl, logger)
	secretHandler := handlers.NewSecretHandler(secretService, projects, projectUsers, tmpl, logger)

	return &Server{
		config:         cfg,
		db:             db,
		logger:         logger,
		providers:      providers,
		users:          users,
		sessions:       sessions,
		projects:       projects,
		projectUsers:   projectUsers,
		builds:         builds,
		templates:      tmpl,
		authHandler:    authHandler,
		projectHandler: projectHandler,
		webhookHandler: webhookHandler,
		buildHandler:   buildHandler,
		secretHandler:  secretHandler,
		authMiddleware: authMiddleware,
	}, nil
}

// Start starts the HTTP server and blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	router := s.setupRoutes()

	// Apply global middleware
	handler := middleware.Chain(
		router,
		middleware.Recovery(s.logger),
		middleware.Logging(s.logger),
		middleware.RequestID,
	)

	s.httpServer = &http.Server{
		Addr:         s.config.BindAddr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("starting HTTP server", "addr", s.config.BindAddr)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	}
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")

	// Create a deadline for shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	s.logger.Info("HTTP server stopped")
	return nil
}
