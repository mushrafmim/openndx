package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/OpenNSW/core/configyaml"
	"github.com/OpenNSW/core/shared/validation"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/provider"
)

// defaultConfigPath is used when neither -config nor CONFIG_PATH is set.
const defaultConfigPath = "config.yaml"

// Config is the OE's configuration, loaded from a single YAML file.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Logging LoggingConfig `yaml:"logging"`
	// SchemaPath is the consumer GraphQL schema file, read once at startup. A
	// relative path is resolved against the directory of the config file.
	SchemaPath string `yaml:"schemaPath"`
	// FoundationalIDArgName is the query argument that identifies the data
	// owner (e.g. "nic"), recognized by name rather than by position.
	FoundationalIDArgName string `yaml:"foundationalIdArgName"`
	// IDP validates each consumer token's signature, issuer, audience and expiry.
	IDP           authn.Config      `yaml:"idp"`
	PDP           ServiceURLConfig  `yaml:"pdp"`
	ConsentEngine ServiceURLConfig  `yaml:"consentEngine"`
	Audit         AuditConfig       `yaml:"audit"`
	Providers     []provider.Config `yaml:"providers"`
	// ArgMappings map consumer query arguments to provider query arguments.
	ArgMappings []*graphql.ArgMapping `yaml:"argMappings"`
}

// ServerConfig holds the HTTP server settings.
type ServerConfig struct {
	Port int `yaml:"port"`
	// Timeout bounds reading a request and writing its response, so it must
	// cover the PDP, consent and provider calls a query makes.
	Timeout time.Duration `yaml:"timeout"`
}

// LoggingConfig holds the log level (debug, info, warn, error) and format (text, json).
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// ServiceURLConfig holds the base URL of a service the OE calls. The OE skips
// the service's check when URL is empty.
type ServiceURLConfig struct {
	URL string `yaml:"url"`
}

// AuditConfig holds the Argus audit service settings. Auditing is off when URL is empty.
type AuditConfig struct {
	URL       string `yaml:"url"`
	APIKey    string `yaml:"apiKey"`
	ActorType string `yaml:"actorType"`
	ActorID   string `yaml:"actorId"`
}

// defaultConfig returns the values used for any key the config file leaves out.
func defaultConfig() Config {
	return Config{
		Server:     ServerConfig{Port: 4000, Timeout: 30 * time.Second},
		Logging:    LoggingConfig{Level: "info", Format: "text"},
		SchemaPath: "schema.graphql",
		Audit:      AuditConfig{ActorType: "SERVICE", ActorID: "orchestration-engine"},
	}
}

// loadConfig reads the config file named by -config, CONFIG_PATH or
// config.yaml, in that order, on top of the defaults and validates the result.
func loadConfig(args []string) (*Config, error) {
	flags := flag.NewFlagSet("orchestration-engine", flag.ContinueOnError)
	path := flags.String("config", configPathFromEnv(), "Path to the YAML config file")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	cfg := defaultConfig()
	if err := configyaml.LoadAndExpand(*path, &cfg); err != nil {
		return nil, err
	}
	if cfg.SchemaPath != "" && !filepath.IsAbs(cfg.SchemaPath) {
		cfg.SchemaPath = filepath.Join(filepath.Dir(*path), cfg.SchemaPath)
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

// Validate checks the configuration before startup. Providers are validated
// when the provider registry is built from them.
func (c *Config) Validate() error {
	if err := validation.TCPPort("server.port", c.Server.Port); err != nil {
		return err
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
	if c.SchemaPath == "" {
		return fmt.Errorf("schemaPath must not be empty")
	}
	if c.FoundationalIDArgName == "" {
		return fmt.Errorf("foundationalIdArgName is required")
	}
	if err := c.IDP.Validate(); err != nil {
		return fmt.Errorf("invalid idp configuration: %w", err)
	}
	for _, service := range []struct{ name, url string }{
		{"pdp.url", c.PDP.URL},
		{"consentEngine.url", c.ConsentEngine.URL},
		{"audit.url", c.Audit.URL},
	} {
		if service.url == "" {
			continue
		}
		if err := validation.HTTPURL(service.name, service.url); err != nil {
			return err
		}
	}
	if c.Audit.ActorType == "" || c.Audit.ActorID == "" {
		return fmt.Errorf("audit.actorType and audit.actorId must not be empty")
	}
	for i, mapping := range c.ArgMappings {
		if mapping == nil || mapping.SchemaID == "" || mapping.TargetArgName == "" ||
			mapping.SourceArgPath == "" || mapping.TargetArgPath == "" {
			return fmt.Errorf("argMappings[%d] must set schemaId, targetArgName, sourceArgPath and targetArgPath", i)
		}
	}
	return nil
}
