package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/oe/configs"
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
	logger.Init()

	// Create context with signal handling for graceful shutdown
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

	// The consumer schema is read once at startup. To change it, deploy a new
	// schema file and restart, so every OE instance serves the same schema.
	consumerSchema, err := schema.Load(config.SchemaPath)
	if err != nil {
		log.Fatalf("Failed to load the consumer schema (check schemaPath in the config file): %v", err)
	}
	logger.Log.Info("Loaded consumer schema", "path", config.SchemaPath)

	// Authenticate consumers: accept tokens the IdP issued for OE's audience, whichever
	// client requested them. IDP_JWKS_URL and IDP_ISSUER default to IDP_BASE_URL's
	// endpoints when unset.
	idpBaseURL := os.Getenv("IDP_BASE_URL")
	authnConfig := authn.Config{
		JWKSURL:               utils.GetEnvOrDefault("IDP_JWKS_URL", idpBaseURL+"/oauth2/jwks"),
		Issuer:                utils.GetEnvOrDefault("IDP_ISSUER", utils.GetEnvOrDefault("IDP_TOKEN_URL", idpBaseURL+"/oauth2/token")),
		Audience:              os.Getenv("IDP_AUDIENCE"),
		InsecureSkipTLSVerify: utils.GetEnvOrDefault("IDP_JWKS_INSECURE_SKIP_VERIFY", "false") == "true",
	}
	if err := authnConfig.Validate(); err != nil {
		log.Fatalf("Invalid authentication configuration (check IDP_JWKS_URL, IDP_ISSUER and IDP_AUDIENCE): %v", err)
	}
	authnManager, err := authn.NewManager(authnConfig)
	if err != nil {
		log.Fatalf("Failed to initialize authentication: %v", err)
	}

	providers, err := provider.NewProviderHandler(config.GetProviders())
	if err != nil {
		log.Fatalf("Invalid provider configuration: %v", err)
	}

	deps := federator.Deps{
		Providers: federator.ProviderLookup(providers.GetProvider),
		Schemas:   consumerSchema,
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
		Federator:    federationObject,
		Authenticate: authnManager.RequireAuthMiddleware(),
	})
}
