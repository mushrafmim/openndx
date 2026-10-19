package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/authz"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/pb/application"
	"github.com/openndx/openndx-core/internal/pb/caller"
	"github.com/openndx/openndx-core/internal/pb/database"
	"github.com/openndx/openndx-core/internal/pb/idp/idpfactory"
	"github.com/openndx/openndx-core/internal/pb/member"
	"github.com/openndx/openndx-core/internal/pb/middleware"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/openndx/openndx-core/internal/pb/schema"
	"github.com/openndx/openndx-core/internal/utils"
)

// Build information - set during build
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	slog.SetDefault(logger)

	slog.Info("Starting Portal Backend initialization",
		"version", Version,
		"build_time", BuildTime,
		"git_commit", GitCommit)

	// Initialize GORM database connection for V1
	v1DbConfig := database.NewDatabaseConfig()
	gormDB, err := database.ConnectGormDB(v1DbConfig)
	if err != nil {
		slog.Error("Failed to connect to GORM database", "error", err)
		os.Exit(1)
	}

	if os.Getenv("RUN_MIGRATION") == "true" {
		if err := member.DropLegacyColumns(gormDB); err != nil {
			slog.Error("Failed to run database migrations", "error", err)
			os.Exit(1)
		}
		err = database.AutoMigrate(gormDB,
			&member.Member{},
			&schema.Schema{},
			&application.Application{},
			&schema.SchemaSubmission{},
			&application.ApplicationSubmission{},
		)
		if err != nil {
			slog.Error("Failed to run database migrations", "error", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Database connected (migration skipped)")
	}

	// Initialize external service clients
	idpProvider, err := idpfactory.NewFromEnv()
	if err != nil {
		slog.Error("Failed to initialize IDP provider", "error", err)
		os.Exit(1)
	}

	pdpClient, err := policy.NewClientFromEnv()
	if err != nil {
		slog.Error("Failed to initialize PDP client", "error", err)
		os.Exit(1)
	}

	// Initialize Audit system
	// Services will work without auditing - gracefully degrades if disabled via ENABLE_AUDIT=false
	// or if AUDIT_SERVICE_URL is not provided
	auditServiceURL := os.Getenv("AUDIT_SERVICE_URL")
	apiKey := os.Getenv("ARGUS_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ARGUS_AUTH_TOKEN")
	}
	auditClient := audit.NewClient(audit.Config{
		BaseURL: auditServiceURL,
		APIKey:  apiKey,
	})
	audit.InitializeGlobalAudit(auditClient)

	// Initialize services and handlers
	memberService := member.NewService(gormDB)
	memberHandler := member.NewHandler(memberService)

	schemaService := schema.NewService(gormDB, pdpClient)
	schemaHandler := schema.NewHandler(schemaService)

	applicationService := application.NewService(gormDB, pdpClient, idpProvider)
	applicationHandler := application.NewHandler(applicationService)

	// Setup authentication: accept tokens the IdP issued for this API's
	// audience, whichever client requested them. IDP_JWKS_URL and IDP_ISSUER
	// default to IDP_BASE_URL's endpoints when unset.
	idpBaseURL := os.Getenv("IDP_BASE_URL")
	authnConfig := authn.Config{
		JWKSURL:               utils.GetEnvOrDefault("IDP_JWKS_URL", idpBaseURL+"/oauth2/jwks"),
		Issuer:                utils.GetEnvOrDefault("IDP_ISSUER", utils.GetEnvOrDefault("IDP_TOKEN_URL", idpBaseURL+"/oauth2/token")),
		Audience:              os.Getenv("IDP_AUDIENCE"),
		InsecureSkipTLSVerify: utils.GetEnvOrDefault("IDP_JWKS_INSECURE_SKIP_VERIFY", "false") == "true",
	}
	if err := authnConfig.Validate(); err != nil {
		slog.Error("Invalid authentication configuration (check IDP_JWKS_URL, IDP_ISSUER and IDP_AUDIENCE)", "error", err)
		os.Exit(1)
	}

	authnManager, err := authn.NewManager(authnConfig)
	if err != nil {
		slog.Error("Failed to initialize authentication", "error", err)
		os.Exit(1)
	}

	// Check each API route's OAuth2 scope against the principal internal/authn validated
	authzr, err := authz.New(caller.PrincipalFromContext)
	if err != nil {
		slog.Error("Failed to initialize authorization", "error", err)
		os.Exit(1)
	}

	// API routes. Each requires one scope: the "-all" scopes never open a
	// route, handlers check them only to widen the caller past its own records.
	apiMux := http.NewServeMux()

	// Member endpoints
	apiMux.Handle("GET /api/v1/members", authzr.RequireScope(caller.ScopeMemberRead)(http.HandlerFunc(memberHandler.GetAllMembers)))
	apiMux.Handle("POST /api/v1/members", authzr.RequireScope(caller.ScopeMemberWriteAll)(http.HandlerFunc(memberHandler.CreateMember)))
	apiMux.Handle("GET /api/v1/members/{memberId}", authzr.RequireScope(caller.ScopeMemberRead)(http.HandlerFunc(memberHandler.GetMember)))
	apiMux.Handle("PUT /api/v1/members/{memberId}", authzr.RequireScope(caller.ScopeMemberWrite)(http.HandlerFunc(memberHandler.UpdateMember)))

	// Schema endpoints
	apiMux.Handle("GET /api/v1/schemas", authzr.RequireScope(caller.ScopeSchemaRead)(http.HandlerFunc(schemaHandler.GetAllSchemas)))
	apiMux.Handle("POST /api/v1/schemas", authzr.RequireScope(caller.ScopeSchemaWrite)(http.HandlerFunc(schemaHandler.CreateSchema)))
	apiMux.Handle("GET /api/v1/schemas/{schemaId}", authzr.RequireScope(caller.ScopeSchemaRead)(http.HandlerFunc(schemaHandler.GetSchema)))
	apiMux.Handle("PUT /api/v1/schemas/{schemaId}", authzr.RequireScope(caller.ScopeSchemaWrite)(http.HandlerFunc(schemaHandler.UpdateSchema)))

	// Schema policy metadata endpoints
	apiMux.Handle("GET /api/v1/schemas/{schemaId}/policy-metadata", authzr.RequireScope(caller.ScopeSchemaRead)(http.HandlerFunc(schemaHandler.ListSchemaPolicyMetadata)))
	apiMux.Handle("PATCH /api/v1/schemas/{schemaId}/policy-metadata/{id}", authzr.RequireScope(caller.ScopeSchemaWrite)(http.HandlerFunc(schemaHandler.PatchSchemaPolicyMetadata)))
	apiMux.Handle("DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}", authzr.RequireScope(caller.ScopeSchemaWrite)(http.HandlerFunc(schemaHandler.DeleteSchemaPolicyMetadata)))
	apiMux.Handle("DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}/allowlist/{applicationId}", authzr.RequireScope(caller.ScopeSchemaWrite)(http.HandlerFunc(schemaHandler.RevokeSchemaPolicyAllowListEntry)))

	// Schema submission endpoints
	apiMux.Handle("GET /api/v1/schema-submissions", authzr.RequireScope(caller.ScopeSchemaSubmissionRead)(http.HandlerFunc(schemaHandler.GetAllSchemaSubmissions)))
	apiMux.Handle("POST /api/v1/schema-submissions", authzr.RequireScope(caller.ScopeSchemaSubmissionWrite)(http.HandlerFunc(schemaHandler.CreateSchemaSubmission)))
	apiMux.Handle("GET /api/v1/schema-submissions/{submissionId}", authzr.RequireScope(caller.ScopeSchemaSubmissionRead)(http.HandlerFunc(schemaHandler.GetSchemaSubmission)))
	apiMux.Handle("PUT /api/v1/schema-submissions/{submissionId}", authzr.RequireScope(caller.ScopeSchemaSubmissionWrite)(http.HandlerFunc(schemaHandler.UpdateSchemaSubmission)))

	// Application endpoints
	apiMux.Handle("GET /api/v1/applications", authzr.RequireScope(caller.ScopeApplicationRead)(http.HandlerFunc(applicationHandler.GetAllApplications)))
	apiMux.Handle("POST /api/v1/applications", authzr.RequireScope(caller.ScopeApplicationWrite)(http.HandlerFunc(applicationHandler.CreateApplication)))
	apiMux.Handle("GET /api/v1/applications/{applicationId}", authzr.RequireScope(caller.ScopeApplicationRead)(http.HandlerFunc(applicationHandler.GetApplication)))
	apiMux.Handle("PUT /api/v1/applications/{applicationId}", authzr.RequireScope(caller.ScopeApplicationWrite)(http.HandlerFunc(applicationHandler.UpdateApplication)))
	apiMux.Handle("PUT /api/v1/applications/{applicationId}/policy", authzr.RequireScope(caller.ScopeApplicationWrite)(http.HandlerFunc(applicationHandler.UpdateApplicationPolicy)))

	// Application submission endpoints
	apiMux.Handle("GET /api/v1/application-submissions", authzr.RequireScope(caller.ScopeApplicationSubmissionRead)(http.HandlerFunc(applicationHandler.GetAllApplicationSubmissions)))
	apiMux.Handle("POST /api/v1/application-submissions", authzr.RequireScope(caller.ScopeApplicationSubmissionWrite)(http.HandlerFunc(applicationHandler.CreateApplicationSubmission)))
	apiMux.Handle("GET /api/v1/application-submissions/{submissionId}", authzr.RequireScope(caller.ScopeApplicationSubmissionRead)(http.HandlerFunc(applicationHandler.GetApplicationSubmission)))
	apiMux.Handle("PUT /api/v1/application-submissions/{submissionId}", authzr.RequireScope(caller.ScopeApplicationSubmissionWrite)(http.HandlerFunc(applicationHandler.UpdateApplicationSubmission)))

	// All incoming traffic. Public routes register here directly; everything
	// under /api/v1/ goes through CORS, authentication and the caller's member
	// lookup to apiMux. The
	// method-less /api/v1/ pattern also catches CORS preflight (OPTIONS)
	// requests, which no apiMux route matches.
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler(gormDB, v1DbConfig.Database))
	mux.HandleFunc("GET /debug", debugHandler)
	mux.HandleFunc("GET /debug/db", debugDBHandler(gormDB, v1DbConfig.Database))

	mux.Handle("/api/v1/", middleware.NewCORSMiddleware()(authnManager.RequireAuthMiddleware()(caller.Middleware(memberService.MemberIDByOuID)(apiMux))))

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	addr := ":" + port
	server := &http.Server{
		Addr:         addr,
		Handler:      utils.PanicRecoveryMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		slog.Info("Portal Backend starting", "port", port, "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start Portal Backend", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down Portal Backend...")

	// Create a deadline to wait for
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	// Gracefully close database connection
	if gormDB != nil {
		if sqlDB, err := gormDB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				slog.Error("Failed to close database connection", "error", err)
			}
		}
	}

	slog.Info("Portal Backend exited")
}
