// Package main is the entry point for FeatherCI.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/featherci/featherci/internal/cache"
	"github.com/featherci/featherci/internal/config"
	"github.com/featherci/featherci/internal/convert"
	"github.com/featherci/featherci/internal/crypto"
	"github.com/featherci/featherci/internal/database"
	"github.com/featherci/featherci/internal/executor"
	"github.com/featherci/featherci/internal/git"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/server"
	"github.com/featherci/featherci/internal/services"
	"github.com/featherci/featherci/internal/status"
	"github.com/featherci/featherci/internal/version"
	"github.com/featherci/featherci/internal/worker"
	"github.com/featherci/featherci/internal/worker/client"
)

func main() {
	// Handle subcommands before flag.Parse()
	if len(os.Args) > 1 && os.Args[1] == "convert" {
		runConvert()
		return
	}

	var (
		showVersion = flag.Bool("version", false, "Print version information and exit")
		generateKey = flag.Bool("generate-key", false, "Generate a secure encryption key and exit")
		devMode     = flag.Bool("dev", false, "Enable development mode (no OAuth required, auto-login as admin)")
		configPath  = flag.String("config", "", "Path to YAML config file (default: /etc/featherci/config.yaml)")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "FeatherCI - Lightweight self-hosted CI/CD\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s convert [directory]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  convert    Convert .github/workflows or .circleci config to FeatherCI format\n")
	}

	flag.Parse()

	if *showVersion {
		fmt.Println(version.Info())
		os.Exit(0)
	}

	if *generateKey {
		key, err := generateSecretKey()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating key: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(key)
		os.Exit(0)
	}

	// Set up context with signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Printf("\nReceived %s, shutting down...\n", sig)
		cancel()
	}()

	if err := run(ctx, *devMode, *configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run is the main application entry point.
func run(ctx context.Context, devMode bool, configPath string) error {
	// Set up structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	fmt.Println(version.Info())

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Apply dev mode settings
	if devMode {
		cfg.DevMode = true
		fmt.Println("** DEVELOPMENT MODE ENABLED **")
		fmt.Println("   - OAuth validation skipped")
		fmt.Println("   - Visit /auth/dev to auto-login as admin")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return err
	}

	fmt.Printf("Mode: %s\n", cfg.Mode)
	fmt.Printf("Bind address: %s\n", cfg.BindAddr)
	fmt.Printf("Base URL: %s\n", cfg.BaseURL)

	// Worker mode: skip DB, create HTTP clients, run worker only
	if cfg.Mode == config.ModeWorker {
		return runWorkerMode(ctx, cfg, logger)
	}

	// Master/Standalone: need database
	fmt.Printf("Database: %s\n", cfg.DatabasePath)
	if !devMode {
		fmt.Printf("OAuth providers: %s\n", strings.Join(cfg.EnabledProviders(), ", "))
		fmt.Printf("Admins: %s\n", strings.Join(cfg.Admins, ", "))
	}

	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	fmt.Println("Database initialized.")

	srv, err := server.New(cfg, db, logger)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Standalone mode: embedded worker + stale worker cleanup
	if cfg.Mode == config.ModeStandalone {
		startEmbeddedWorker(ctx, cfg, db, logger)
	}

	// Master mode: stale worker cleanup only (no embedded worker)
	if cfg.Mode == config.ModeMaster {
		startStaleWorkerCleanup(ctx, db, logger)
	}

	fmt.Printf("\nFeatherCI is running at %s\n", cfg.BaseURL)
	return srv.Start(ctx)
}

// startEmbeddedWorker launches the embedded worker for standalone mode.
func startEmbeddedWorker(ctx context.Context, cfg *config.Config, db *database.DB, logger *slog.Logger) {
	dockerExec, err := executor.NewDockerExecutor()
	if err != nil {
		slog.Error("failed to create docker executor", "error", err)
		return
	}
	cacheManager := cache.NewCacheManager(cfg.CachePath)
	stepRunner := executor.NewStepRunner(dockerExec, cacheManager)
	gitSvc := git.NewCLIGitService()
	wsMgr := git.NewWorkspaceManager(cfg.WorkspacePath)

	stepRepo := models.NewBuildStepRepository(db.DB)
	buildRepo := models.NewBuildRepository(db.DB)
	projectRepo := models.NewProjectRepository(db.DB)
	workerRepo := models.NewWorkerRepository(db.DB)
	projectUserRepo := models.NewProjectUserRepository(db.DB)
	tokenSrc := worker.NewProjectTokenSource(projectUserRepo)

	encryptor, err := crypto.NewEncryptor(cfg.SecretKey)
	if err != nil {
		slog.Error("failed to create encryptor", "error", err)
		return
	}
	secretRepo := models.NewSecretRepository(db.DB)
	secretSvc := services.NewSecretService(secretRepo, encryptor)

	notificationRepo := models.NewNotificationChannelRepository(db.DB)
	notificationSvc := services.NewNotificationService(notificationRepo, stepRepo, encryptor, cfg.BaseURL, cfg.DevMode, logger)
	statusSvc := status.NewStatusService(cfg, tokenSrc, logger)

	workerCfg := worker.DefaultConfig()
	workerCfg.MaxConcurrent = cfg.MaxConcurrent
	advancer := services.NewBuildAdvancer(stepRepo, buildRepo, projectRepo, statusSvc, notificationSvc, logger)
	w := worker.New(workerCfg, stepRepo, buildRepo, projectRepo, workerRepo, tokenSrc, secretSvc, gitSvc, wsMgr, stepRunner, statusSvc, notificationSvc, advancer, worker.NoopLogUploader{}, logger)
	w.SetID("embedded")

	go func() {
		if err := w.Start(ctx); err != nil {
			slog.Error("worker stopped with error", "error", err)
		}
	}()

	// Cache cleanup goroutine
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := cacheManager.Cleanup(7 * 24 * time.Hour); err != nil {
					slog.Error("cache cleanup failed", "error", err)
				}
			}
		}
	}()

	// Stale worker cleanup
	startStaleWorkerCleanup(ctx, db, logger)
}

// startStaleWorkerCleanup launches the goroutine that detects and cleans up stale workers.
func startStaleWorkerCleanup(ctx context.Context, db *database.DB, _ *slog.Logger) {
	workerRepo := models.NewWorkerRepository(db.DB)
	stepRepo := models.NewBuildStepRepository(db.DB)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stale, err := workerRepo.ListStale(ctx, 60*time.Second)
				if err != nil {
					slog.Error("failed to list stale workers", "error", err)
					continue
				}
				for _, sw := range stale {
					slog.Warn("resetting stale worker", "worker_id", sw.ID)
					if err := stepRepo.ResetStepsForWorker(ctx, sw.ID); err != nil {
						slog.Error("failed to reset steps for stale worker", "worker_id", sw.ID, "error", err)
					}
					if err := workerRepo.SetOffline(ctx, sw.ID); err != nil {
						slog.Error("failed to set stale worker offline", "worker_id", sw.ID, "error", err)
					}
				}
				if purged, err := workerRepo.PurgeOffline(ctx, 24*time.Hour); err != nil {
					slog.Error("failed to purge offline workers", "error", err)
				} else if purged > 0 {
					slog.Info("purged old offline workers", "count", purged)
				}
			}
		}
	}()
}

// runWorkerMode starts a standalone worker that communicates with the master over HTTP.
func runWorkerMode(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	fmt.Printf("Master URL: %s\n", cfg.MasterURL)

	dockerExec, err := executor.NewDockerExecutor()
	if err != nil {
		return fmt.Errorf("failed to create docker executor: %w", err)
	}
	cacheManager := cache.NewCacheManager(cfg.CachePath)
	stepRunner := executor.NewStepRunner(dockerExec, cacheManager)
	gitSvc := git.NewCLIGitService()
	wsMgr := git.NewWorkspaceManager(cfg.WorkspacePath)

	// Create HTTP client for master API
	apiClient := client.New(cfg.MasterURL, cfg.WorkerSecret)

	workerCfg := worker.DefaultConfig()
	workerCfg.MaxConcurrent = cfg.MaxConcurrent
	w := worker.New(
		workerCfg,
		apiClient.StepClient(),
		apiClient.BuildClient(),
		apiClient.ProjectClient(),
		apiClient.WorkerClient(),
		apiClient,                  // tokenSource
		apiClient,                  // secretSource
		gitSvc,
		wsMgr,
		stepRunner,
		client.NoopStatusPoster{},  // master handles statuses
		client.NoopNotifier{},      // master handles notifications
		client.NoopAdvancer{},      // master handles advancement
		apiClient,                  // logUploader
		logger,
	)

	go func() {
		if err := w.Start(ctx); err != nil {
			slog.Error("worker stopped with error", "error", err)
		}
	}()

	// Cache cleanup goroutine
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := cacheManager.Cleanup(7 * 24 * time.Hour); err != nil {
					slog.Error("cache cleanup failed", "error", err)
				}
			}
		}
	}()

	fmt.Printf("\nFeatherCI worker is running (master: %s)\n", cfg.MasterURL)

	// Block until context is cancelled
	<-ctx.Done()
	return nil
}

// runConvert handles the "featherci convert" subcommand.
func runConvert() {
	dir := "."
	if len(os.Args) > 2 {
		dir = os.Args[2]
	}

	if err := convert.Run(dir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// generateSecretKey generates a cryptographically secure 32-byte key
// encoded as base64 for use as FEATHERCI_SECRET_KEY.
func generateSecretKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
