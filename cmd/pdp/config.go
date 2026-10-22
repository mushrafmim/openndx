package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/OpenNSW/core/configyaml"
	"github.com/OpenNSW/core/database"
)

// defaultConfigPath is used when neither -config nor CONFIG_PATH is set.
const defaultConfigPath = "config.yaml"

// Config is the PDP's configuration, loaded from a single YAML file.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Logging  LoggingConfig  `yaml:"logging"`
	Database DatabaseConfig `yaml:"database"`
}

// ServerConfig holds the HTTP server settings.
type ServerConfig struct {
	Port int `yaml:"port"`
	// Timeout bounds reading a request and writing its response.
	Timeout time.Duration `yaml:"timeout"`
}

// LoggingConfig holds the log level (debug, info, warn, error) and format (text, json).
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// DatabaseConfig is the shared database configuration plus the PDP's own settings.
type DatabaseConfig struct {
	database.Config `yaml:",inline"`
	// IdleTimeout closes pooled connections that stay idle this long.
	IdleTimeout time.Duration `yaml:"idleTimeout"`
	// RunMigration creates the enum types and the policy_metadata table at startup.
	RunMigration bool `yaml:"runMigration"`
}

// defaultConfig returns the values used for any key the config file leaves out.
func defaultConfig() Config {
	return Config{
		Server:  ServerConfig{Port: 8082, Timeout: 10 * time.Second},
		Logging: LoggingConfig{Level: "info", Format: "text"},
		Database: DatabaseConfig{
			Config: database.Config{
				Driver: database.Postgres,
				Postgres: &database.PostgresConfig{
					Port:    5432,
					Name:    "pdp",
					SSLMode: "require",
					Pool:    database.PoolConfig{MaxOpenConns: 25, MaxIdleConns: 5, MaxConnLifetimeSeconds: 3600},
				},
			},
			IdleTimeout: 30 * time.Minute,
		},
	}
}

// loadConfig reads the config file named by -config, CONFIG_PATH or
// config.yaml, in that order, on top of the defaults and validates the result.
func loadConfig(args []string) (*Config, error) {
	flags := flag.NewFlagSet("policy-decision-point", flag.ContinueOnError)
	path := flags.String("config", configPathFromEnv(), "Path to the YAML config file")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	cfg := defaultConfig()
	if err := configyaml.LoadAndExpand(*path, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config file %s: %w", *path, err)
	}
	return &cfg, nil
}

// configPathFromEnv returns CONFIG_PATH, or config.yaml when it is unset.
func configPathFromEnv() string {
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		return path
	}
	return defaultConfigPath
}

// Validate checks the configuration before startup.
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if c.Server.Timeout <= 0 {
		return fmt.Errorf("server.timeout must be positive")
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be debug, info, warn or error")
	}
	switch c.Logging.Format {
	case "text", "json":
	default:
		return fmt.Errorf("logging.format must be text or json")
	}
	if c.Database.Driver != database.Postgres {
		return fmt.Errorf("database.driver must be postgres")
	}
	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("invalid database configuration: %w", err)
	}
	pg := c.Database.Postgres
	if pg.Port < 1 || pg.Port > 65535 {
		return fmt.Errorf("database.postgres.port must be between 1 and 65535")
	}
	if pg.SSLMode == "" {
		return fmt.Errorf("database.postgres.sslMode is required")
	}
	if pg.Pool.MaxOpenConns <= 0 || pg.Pool.MaxIdleConns <= 0 || pg.Pool.MaxIdleConns > pg.Pool.MaxOpenConns || pg.Pool.MaxConnLifetimeSeconds <= 0 {
		return fmt.Errorf("invalid database.postgres.pool: require positive connection limits and lifetime, with maxIdleConns no greater than maxOpenConns")
	}
	if c.Database.IdleTimeout <= 0 {
		return fmt.Errorf("database.idleTimeout must be positive")
	}
	return nil
}
