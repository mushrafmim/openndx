// Package config provides simplified configuration management
package config

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/OpenNSW/core/database"
	"github.com/openndx/openndx-core/internal/utils"
)

// Config holds all configuration for a service
type Config struct {
	Environment         string
	ConsentPortalUrl    string
	Service             ServiceConfig
	Logging             LoggingConfig
	Security            SecurityConfig
	IDPConfig           IDPConfig
	Database            database.Config
	DatabaseIdleTimeout time.Duration
}

// ServiceConfig holds service-specific configuration
type ServiceConfig struct {
	Name           string
	Port           string
	Host           string
	Timeout        time.Duration
	AllowedOrigins string
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string
	Format string
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	EnableCORS bool
	RateLimit  int
}

// IDPConfig holds IDP configuration
type IDPConfig struct {
	Issuer   string
	JwksUrl  string
	Audience string
	ClientID string
	// SubjectClaim is the token claim carrying the data owner's canonical UID
	// (matched against the consent record's owner_id). Defaults to "sub".
	SubjectClaim string
	// InsecureSkipVerify skips TLS verification on the JWKS fetch. Dev-only
	// (e.g. a self-signed local IdP); leave false in production.
	InsecureSkipVerify bool
}

// LoadConfig loads configuration from flags and environment variables
func LoadConfig(serviceName string) (*Config, error) {
	return loadConfig(serviceName, os.Args[1:])
}

func loadConfig(serviceName string, args []string) (*Config, error) {
	// Get environment first to determine defaults
	env := utils.GetEnvOrDefault("ENVIRONMENT", "local")

	// A local FlagSet keeps loading independent of package-global flag state.
	flags := flag.NewFlagSet(serviceName, flag.ContinueOnError)

	// Define flags
	envFlag := flags.String("env", env, "Environment: local or production")
	port := flags.String("port", utils.GetEnvOrDefault("PORT", "8081"), "Service port")
	host := flags.String("host", utils.GetEnvOrDefault("HOST", "0.0.0.0"), "Host address")
	timeout := flags.Duration("timeout", 10*time.Second, "Request timeout")
	logLevel := flags.String("log-level", getDefaultLogLevel(env), "Log level")
	logFormat := flags.String("log-format", getDefaultLogFormat(env), "Log format")
	enableCORS := flags.Bool("cors", getDefaultCORS(env), "Enable CORS")
	rateLimit := flags.Int("rate-limit", getDefaultRateLimit(env), "Rate limit per minute")

	// Parse flags
	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	// Reading IDP Configs (generic across IdPs; each check is applied by the
	// verifier only when its value is configured)
	userIssuer := utils.GetEnvOrDefault("IDP_ISSUER", "")
	userAudience := utils.GetEnvOrDefault("IDP_AUDIENCE", "")
	userJwksURL := utils.GetEnvOrDefault("IDP_JWKS_URL", "")
	userClientID := utils.GetEnvOrDefault("IDP_CLIENT_ID", "")
	// Token claim carrying the owner UID matched against consent owner_id.
	userSubjectClaim := utils.GetEnvOrDefault("IDP_SUBJECT_CLAIM", "sub")
	// Dev-only: skip TLS verification on the JWKS fetch (self-signed local IdP).
	jwksInsecureSkipVerify, err := strconv.ParseBool(utils.GetEnvOrDefault("IDP_JWKS_INSECURE_SKIP_VERIFY", "false"))
	if err != nil {
		return nil, fmt.Errorf("IDP_JWKS_INSECURE_SKIP_VERIFY must be a boolean")
	}

	// Reading DB Configs
	dbHost := utils.GetEnvOrDefault("DB_HOST", "localhost")
	dbPort, err := intEnv("DB_PORT", "5432")
	if err != nil {
		return nil, err
	}
	dbUsername := utils.GetEnvOrDefault("DB_USERNAME", "postgres")
	dbPassword := utils.GetEnvOrDefault("DB_PASSWORD", "")
	dbName := utils.GetEnvOrDefault("DB_NAME", "consent_engine")
	dbSslMode := utils.GetEnvOrDefault("DB_SSLMODE", "require")

	maxOpen, err := intEnv("DB_MAX_OPEN_CONNS", "25")
	if err != nil {
		return nil, err
	}
	maxIdle, err := intEnv("DB_MAX_IDLE_CONNS", "5")
	if err != nil {
		return nil, err
	}
	lifetime, err := intEnv("DB_CONN_MAX_LIFETIME_SECONDS", "3600")
	if err != nil {
		return nil, err
	}
	idleTimeout, err := time.ParseDuration(utils.GetEnvOrDefault("DB_CONN_MAX_IDLE_TIME", "30m"))
	if err != nil {
		return nil, fmt.Errorf("DB_CONN_MAX_IDLE_TIME must be a duration: %w", err)
	}

	// Reading ConsentPortal Url
	consentPortalUrl := utils.GetEnvOrDefault("CONSENT_PORTAL_URL", "http://localhost:5173")
	allowedOrigins := utils.GetEnvOrDefault("CORS_ALLOWED_ORIGINS", "")

	// add the consent portal url to the allowed origins list
	allowedOrigins += "," + consentPortalUrl

	// Use flag value if provided, otherwise use environment default
	finalEnv := *envFlag

	config := &Config{
		Environment:      finalEnv,
		ConsentPortalUrl: consentPortalUrl,
		Service: ServiceConfig{
			Name:           serviceName,
			Port:           *port,
			Host:           *host,
			Timeout:        *timeout,
			AllowedOrigins: allowedOrigins,
		},
		Logging: LoggingConfig{
			Level:  *logLevel,
			Format: *logFormat,
		},
		Security: SecurityConfig{
			EnableCORS: *enableCORS,
			RateLimit:  *rateLimit,
		},
		IDPConfig: IDPConfig{
			Issuer:             userIssuer,
			JwksUrl:            userJwksURL,
			Audience:           userAudience,
			ClientID:           userClientID,
			SubjectClaim:       userSubjectClaim,
			InsecureSkipVerify: jwksInsecureSkipVerify,
		},
		Database: database.Config{
			Driver: database.Postgres,
			Postgres: &database.PostgresConfig{
				Host: dbHost, Port: dbPort, User: dbUsername, Password: dbPassword,
				Name: dbName, SSLMode: dbSslMode,
				Pool: database.PoolConfig{
					MaxOpenConns: maxOpen, MaxIdleConns: maxIdle, MaxConnLifetimeSeconds: lifetime,
				},
			},
		},
		DatabaseIdleTimeout: idleTimeout,
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

func getDefaultLogLevel(env string) string {
	if env == "production" {
		return "warn"
	}
	return "debug"
}

func getDefaultLogFormat(env string) string {
	if env == "production" {
		return "json"
	}
	return "text"
}

func getDefaultCORS(env string) bool {
	return env != "production"
}

func getDefaultRateLimit(env string) int {
	if env == "production" {
		return 100
	}
	return 1000
}

// Validate checks the supplied application configuration before startup.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("configuration is required")
	}
	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("invalid database configuration: %w", err)
	}
	if c.Database.Driver != database.Postgres {
		return fmt.Errorf("only PostgreSQL is supported by the consent schema")
	}
	pg := c.Database.Postgres
	if pg.Port < 1 || pg.Port > 65535 {
		return fmt.Errorf("DB_PORT must be between 1 and 65535")
	}
	if pg.SSLMode == "" {
		return fmt.Errorf("database SSL mode is required")
	}
	if pg.Pool.MaxOpenConns <= 0 || pg.Pool.MaxIdleConns <= 0 || pg.Pool.MaxIdleConns > pg.Pool.MaxOpenConns || pg.Pool.MaxConnLifetimeSeconds <= 0 {
		return fmt.Errorf("invalid database pool settings: require positive connection limits and lifetime, with max idle no greater than max open")
	}
	if c.DatabaseIdleTimeout <= 0 {
		return fmt.Errorf("database idle timeout must be positive")
	}
	port, err := strconv.Atoi(c.Service.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("service port must be an integer between 1 and 65535")
	}
	if c.Service.Timeout <= 0 {
		return fmt.Errorf("service timeout must be positive")
	}
	portal, err := url.Parse(c.ConsentPortalUrl)
	if err != nil || portal.Scheme == "" || portal.Host == "" {
		return fmt.Errorf("consent portal URL must include a scheme and host")
	}
	return nil
}

func intEnv(name, defaultValue string) (int, error) {
	value, err := strconv.Atoi(utils.GetEnvOrDefault(name, defaultValue))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return value, nil
}
