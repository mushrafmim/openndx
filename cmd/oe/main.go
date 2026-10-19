package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LSFLK/argus/pkg/audit"
	coredb "github.com/OpenNSW/core/database"
	"github.com/openndx/openndx-core/internal/database"
	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/configs"
	"github.com/openndx/openndx-core/internal/oe/consent"
	oedatabase "github.com/openndx/openndx-core/internal/oe/database"
	"github.com/openndx/openndx-core/internal/oe/federator"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/handlers"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/middleware"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
	"github.com/openndx/openndx-core/internal/oe/server"
	"github.com/openndx/openndx-core/internal/oe/services"
)

func main() {
	logger.Init()

	// Create context with signal handling for graceful shutdown
	// This ensures background goroutines (like JWKS refresh) are properly cancelled
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Load configuration with proper error handling
	config, err := configs.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize the audit client
	// Allow environment variable override for runtime configuration
	envAuditURL := os.Getenv("AUDIT_SERVICE_URL")
	if envAuditURL != "" {
		config.AuditConfig.ServiceURL = envAuditURL
	}
	apiKey := os.Getenv("ARGUS_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ARGUS_AUTH_TOKEN")
	}
	auditClient := audit.NewClient(audit.Config{
		BaseURL: config.AuditConfig.ServiceURL,
		APIKey:  apiKey,
	})

	// The schema database is optional; without it the active schema comes from config
	// and the schema management routes report the service as unavailable.
	dbConfig, dbIdleTimeout, err := configs.DatabaseFromEnv()
	if err != nil {
		log.Fatalf("Invalid database configuration: %v", err)
	}
	var schemaStore schema.Store
	var schemaService handlers.SchemaService
	if schemaDB, sqlDB, err := openSchemaDB(ctx, dbConfig, dbIdleTimeout); err != nil {
		logger.Log.Error("Failed to connect to database", "error", err)
		logger.Log.Warn("Running without database - schema management disabled")
	} else {
		defer sqlDB.Close()
		svc := services.NewSchemaService(schemaDB)
		schemaStore = svc
		schemaService = svc
	}
	var configSDL string
	if config.Schema != nil {
		configSDL = *config.Schema
	}

	// Validate JWT configuration based on trustUpstream setting
	tokenValidator, err := auth.NewTokenValidatorFromConfig(ctx, config)
	if err != nil {
		log.Fatalf("Failed to initialize token validator: %v", err)
	}

	deps := federator.Deps{
		Providers: federator.ProviderLookup(provider.NewProviderHandler(config.GetProviders()).GetProvider),
		Schemas:   schema.NewSource(schemaStore, configSDL),
		Auditor:   middleware.NewAuditor(auditClient, config.AuditConfig.ActorType, config.AuditConfig.ActorID),
	}
	if config.PdpConfig.ClientURL != "" {
		deps.Policy = policy.NewPdpClient(config.PdpConfig.ClientURL)
	}
	if config.CeConfig.ClientURL != "" {
		deps.Consent = consent.NewCEServiceClient(config.CeConfig.ClientURL)
	}

	federationObject, err := federator.New(deps, federator.OptionsFromConfig(config))
	if err != nil {
		log.Fatalf("Failed to initialize federator: %v", err)
	}

	// Run server with graceful shutdown support
	// Server will stop when ctx is cancelled (on SIGINT/SIGTERM)
	server.RunServer(ctx, server.Deps{
		Config:         config,
		Federator:      federationObject,
		SchemaService:  schemaService,
		TokenValidator: tokenValidator,
	})
}

// openSchemaDB opens the shared PostgreSQL pool and the schema store on top of it.
// The caller must close the returned pool on shutdown.
func openSchemaDB(ctx context.Context, cfg coredb.Config, idleTimeout time.Duration) (*oedatabase.SchemaDB, *sql.DB, error) {
	sqlDB, err := database.Open(ctx, cfg, idleTimeout)
	if err != nil {
		return nil, nil, err
	}
	schemaDB, err := oedatabase.NewSchemaDB(sqlDB)
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	return schemaDB, sqlDB, nil
}
