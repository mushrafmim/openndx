package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	coredb "github.com/OpenNSW/core/database"
	_ "github.com/jackc/pgx/v5/stdlib" // Register the PostgreSQL driver for coredb.New.
	"github.com/openndx/openndx-core/internal/ce/auth"
	"github.com/openndx/openndx-core/internal/ce/config"
	"github.com/openndx/openndx-core/internal/ce/database"
	"github.com/openndx/openndx-core/internal/ce/handlers"
	"github.com/openndx/openndx-core/internal/ce/router"
	"github.com/openndx/openndx-core/internal/ce/services"
	"github.com/openndx/openndx-core/internal/monitoring"
	"github.com/openndx/openndx-core/internal/utils"
)

// Build information - set during build
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Consent engine failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration using flags
	cfg, err := config.LoadConfig("consent-engine")
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// Setup logging
	utils.SetupLogging(cfg.Logging.Format, cfg.Logging.Level)

	slog.Info("Starting consent engine",
		"environment", cfg.Environment,
		"port", cfg.Service.Port,
		"version", Version,
		"build_time", BuildTime,
		"git_commit", GitCommit)

	// OpenNSW owns the SQL pool used by the consent service.
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()
	v1SqlDB, err := coredb.New(startupCtx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer func() {
		if err := v1SqlDB.Close(); err != nil {
			slog.Error("Failed to close V1 database connection", "error", err)
		} else {
			slog.Info("V1 database connection closed successfully")
		}
	}()
	v1SqlDB.SetConnMaxIdleTime(cfg.DatabaseIdleTimeout)
	cancelStartup()

	// Temporary adapter until #631 replaces the service's GORM dependency.
	v1DB, err := database.Wrap(v1SqlDB)
	if err != nil {
		return fmt.Errorf("initialize temporary GORM adapter: %w", err)
	}

	// Get consent portal URL from environment
	slog.Info("Using consent portal URL", "url", cfg.ConsentPortalUrl)

	// Initialize V1 consent service
	v1ConsentService, err := services.NewConsentService(v1DB, cfg.ConsentPortalUrl)
	if err != nil {
		return fmt.Errorf("initialize consent service: %w", err)
	}

	// Initialize V1 handlers
	v1InternalHandler := handlers.NewInternalHandler(v1ConsentService)
	v1PortalHandler := handlers.NewPortalHandler(v1ConsentService)

	slog.Info("JWT verifier configuration",
		"client_id", cfg.IDPConfig.ClientID,
		"issuer", cfg.IDPConfig.Issuer,
		"audience", cfg.IDPConfig.Audience,
		"jwks_url", cfg.IDPConfig.JwksUrl,
		"subject_claim", cfg.IDPConfig.SubjectClaim,
		"jwks_insecure_skip_verify", cfg.IDPConfig.InsecureSkipVerify)

	v1JWTVerifier, err := auth.NewJWTVerifier(auth.JWTVerifierConfig{
		JWKSUrl:            cfg.IDPConfig.JwksUrl,
		Issuer:             cfg.IDPConfig.Issuer,
		Audience:           cfg.IDPConfig.Audience,
		ClientID:           cfg.IDPConfig.ClientID,
		SubjectClaim:       cfg.IDPConfig.SubjectClaim,
		InsecureSkipVerify: cfg.IDPConfig.InsecureSkipVerify,
	})
	if err != nil {
		return fmt.Errorf("initialize JWT verifier: %w", err)
	}

	// Initialize V1 router and register all V1 routes
	v1Router := router.NewV1Router(cfg.Service.AllowedOrigins, v1InternalHandler, v1PortalHandler, v1JWTVerifier)
	mux := http.NewServeMux()

	slog.Info("Registering V1 API routes")
	v1Router.RegisterRoutes(mux)
	slog.Info("V1 API routes registered successfully")

	// Register legacy /health endpoint for compatibility with health checks
	mux.Handle("/health", utils.PanicRecoveryMiddleware(utils.HealthHandler("consent-engine")))

	// Register /metrics endpoint for Prometheus scraping
	mux.Handle("/metrics", monitoring.Handler())

	// Create server configuration
	serverConfig := &utils.ServerConfig{
		Port:         cfg.Service.Port,
		ReadTimeout:  cfg.Service.Timeout,
		WriteTimeout: cfg.Service.Timeout,
		IdleTimeout:  60 * time.Second,
	}

	// Wrap the mux with metrics (outermost) and then CORS from v1 router
	// Metrics must be outermost to capture all requests, including CORS-blocked ones
	handler := monitoring.HTTPMetricsMiddleware(v1Router.ApplyCORS(mux))
	httpServer := utils.CreateServer(serverConfig, handler)

	// Start server with graceful shutdown
	if err := utils.StartServerWithGracefulShutdown(httpServer, "consent-engine"); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}
