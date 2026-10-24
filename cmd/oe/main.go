package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/middleware"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
	"github.com/openndx/openndx-core/internal/oe/server"
	"github.com/openndx/openndx-core/internal/utils"
)

func main() {
	// Create context with signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	config, err := loadConfig(os.Args[1:])
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	utils.SetupLogging(config.Logging.Format, config.Logging.Level)
	logger.Log = slog.Default()

	auditClient := audit.NewClient(audit.Config{
		BaseURL: config.Audit.URL,
		APIKey:  config.Audit.APIKey,
	})

	// The consumer schema is read once at startup. To change it, deploy a new
	// schema file and restart, so every OE instance serves the same schema.
	consumerSchema, err := schema.Load(config.SchemaPath)
	if err != nil {
		log.Fatalf("Failed to load the consumer schema (check schemaPath in the config file): %v", err)
	}
	logger.Log.Info("Loaded consumer schema", "path", config.SchemaPath)

	// Authenticate consumers: accept tokens the IdP issued for OE's audience,
	// whichever client requested them.
	authnManager, err := authn.NewManager(config.IDP)
	if err != nil {
		log.Fatalf("Failed to initialize authentication: %v", err)
	}

	providers, err := provider.NewRegistry(config.Providers...)
	if err != nil {
		log.Fatalf("Invalid provider configuration: %v", err)
	}

	deps := federator.Deps{
		Providers: federator.ProviderLookup(providers.Get),
		Schemas:   consumerSchema,
		Auditor:   middleware.NewAuditor(auditClient, config.Audit.ActorType, config.Audit.ActorID),
	}
	if config.PDP.URL != "" {
		deps.Policy = policy.NewPdpClient(config.PDP.URL)
	}
	if config.ConsentEngine.URL != "" {
		deps.Consent = consent.NewCEServiceClient(config.ConsentEngine.URL)
	}

	federationObject, err := federator.New(deps, federator.Options{
		FoundationalIdArgName: config.FoundationalIDArgName,
		ArgMapping:            config.ArgMappings,
	})
	if err != nil {
		log.Fatalf("Failed to initialize federator: %v", err)
	}

	// Run server with graceful shutdown support
	// Server will stop when ctx is cancelled (on SIGINT/SIGTERM)
	server.RunServer(ctx, config.Server.Port, config.Server.Timeout, server.Deps{
		Federator:    federationObject,
		Authenticate: authnManager.RequireAuthMiddleware(),
	})
}
