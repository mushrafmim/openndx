package configs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/pkg/auth"
)

func TestLoadConfigFromBytes_ValidJSON(t *testing.T) {
	jsonData := []byte(`{
		"ceUrl": "http://ce.example.com",
		"pdpUrl": "http://pdp.example.com",
		"providers": [
			{
				"providerUrl": "http://provider1.example.com",
				"schemaId": "schema1"
			}
		],
		"argMappings": [
			{
				"sourceArg": "arg1",
				"targetArg": "target1"
			}
		],
		"server": {
			"port": "8080"
		},
		"log": {
			"level": "info"
		}
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config == nil {
		t.Fatal("Expected config to be non-nil")
	}
	if config.CeURL != "http://ce.example.com" {
		t.Errorf("Expected CeURL to be 'http://ce.example.com', got '%s'", config.CeURL)
	}
	if config.PdpURL != "http://pdp.example.com" {
		t.Errorf("Expected PdpURL to be 'http://pdp.example.com', got '%s'", config.PdpURL)
	}
	if config.Server.Port != "8080" {
		t.Errorf("Expected Server.Port to be '8080', got '%s'", config.Server.Port)
	}
	if config.Log.Level != "info" {
		t.Errorf("Expected Log.Level to be 'info', got '%s'", config.Log.Level)
	}
	if len(config.Providers) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(config.Providers))
	}
	if config.Providers[0].SchemaID != "schema1" {
		t.Errorf("Expected SchemaID to be 'schema1', got '%s'", config.Providers[0].SchemaID)
	}
}

func TestLoadConfigFromBytes_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json}`)

	config, err := LoadConfigFromBytes(invalidJSON)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if config != nil {
		t.Errorf("Expected config to be nil, got %v", config)
	}
	if !strings.Contains(err.Error(), "error unmarshaling config JSON") {
		t.Errorf("Expected error message to contain 'error unmarshaling config JSON', got '%s'", err.Error())
	}
}

func TestLoadConfigFromBytes_EmptyJSON(t *testing.T) {
	emptyJSON := []byte(`{}`)

	config, err := LoadConfigFromBytes(emptyJSON)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config == nil {
		t.Fatal("Expected config to be non-nil")
	}
	if config.CeURL != "" {
		t.Errorf("Expected CeURL to be empty, got '%s'", config.CeURL)
	}
	if config.PdpURL != "" {
		t.Errorf("Expected PdpURL to be empty, got '%s'", config.PdpURL)
	}
}

func TestLoadConfigFromBytes_DerivedConfigLogic_PdpURL(t *testing.T) {
	jsonData := []byte(`{
		"pdpUrl": "http://pdp.example.com"
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config.PdpURL != "http://pdp.example.com" {
		t.Errorf("Expected PdpURL to be 'http://pdp.example.com', got '%s'", config.PdpURL)
	}
	if config.PdpConfig.ClientURL != "http://pdp.example.com" {
		t.Errorf("Expected PdpConfig.ClientURL to be 'http://pdp.example.com', got '%s'", config.PdpConfig.ClientURL)
	}
}

func TestLoadConfigFromBytes_DerivedConfigLogic_CeURL(t *testing.T) {
	jsonData := []byte(`{
		"ceUrl": "http://ce.example.com"
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config.CeURL != "http://ce.example.com" {
		t.Errorf("Expected CeURL to be 'http://ce.example.com', got '%s'", config.CeURL)
	}
	if config.CeConfig.ClientURL != "http://ce.example.com" {
		t.Errorf("Expected CeConfig.ClientURL to be 'http://ce.example.com', got '%s'", config.CeConfig.ClientURL)
	}
}

func TestLoadConfigFromBytes_DerivedConfigLogic_PdpConfigTakesPrecedence(t *testing.T) {
	jsonData := []byte(`{
		"pdpUrl": "http://pdp.example.com",
		"pdpConfig": {
			"clientUrl": "http://custom-pdp.example.com"
		}
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config.PdpURL != "http://pdp.example.com" {
		t.Errorf("Expected PdpURL to be 'http://pdp.example.com', got '%s'", config.PdpURL)
	}
	if config.PdpConfig.ClientURL != "http://custom-pdp.example.com" {
		t.Errorf("Expected PdpConfig.ClientURL to be 'http://custom-pdp.example.com', got '%s'", config.PdpConfig.ClientURL)
	}
}

func TestLoadConfigFromBytes_DerivedConfigLogic_CeConfigTakesPrecedence(t *testing.T) {
	jsonData := []byte(`{
		"ceUrl": "http://ce.example.com",
		"ceConfig": {
			"clientUrl": "http://custom-ce.example.com"
		}
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config.CeURL != "http://ce.example.com" {
		t.Errorf("Expected CeURL to be 'http://ce.example.com', got '%s'", config.CeURL)
	}
	if config.CeConfig.ClientURL != "http://custom-ce.example.com" {
		t.Errorf("Expected CeConfig.ClientURL to be 'http://custom-ce.example.com', got '%s'", config.CeConfig.ClientURL)
	}
}

func TestLoadConfigFromBytes_ArgMappingFallback(t *testing.T) {
	jsonData := []byte(`{
		"argMappings": [
			{
				"sourceArg": "arg1",
				"targetArg": "target1"
			}
		]
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config.ArgMapping == nil {
		t.Fatal("Expected ArgMapping to be non-nil")
	}
	if len(config.ArgMapping) != len(config.ArgMappings) {
		t.Errorf("Expected ArgMapping to equal ArgMappings")
	}
}

func TestLoadConfigFromBytes_ArgMappingExplicitlySet(t *testing.T) {
	jsonData := []byte(`{
		"argMappings": [
			{
				"sourceArg": "arg1",
				"targetArg": "target1"
			}
		],
		"argMapping": [
			{
				"sourceArg": "arg2",
				"targetArg": "target2"
			}
		]
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config.ArgMapping == nil {
		t.Fatal("Expected ArgMapping to be non-nil")
	}
	if len(config.ArgMapping) != 1 {
		t.Errorf("Expected ArgMapping length to be 1, got %d", len(config.ArgMapping))
	}
	if len(config.ArgMappings) != 1 {
		t.Errorf("Expected ArgMappings length to be 1, got %d", len(config.ArgMappings))
	}
}

func TestLoadConfigFile_SchemaPath(t *testing.T) {
	absolute := filepath.Join(t.TempDir(), "schemas", "consumer.graphql")
	tests := []struct {
		name       string
		schemaPath string
		want       func(configDir string) string
	}{
		{
			name: "defaults to schema.graphql next to the config file",
			want: func(configDir string) string { return filepath.Join(configDir, "schema.graphql") },
		},
		{
			name:       "resolves a relative path against the config file's directory",
			schemaPath: "schemas/consumer.graphql",
			want:       func(configDir string) string { return filepath.Join(configDir, "schemas", "consumer.graphql") },
		},
		{
			name:       "keeps an absolute path",
			schemaPath: absolute,
			want:       func(string) string { return absolute },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			configPath := filepath.Join(configDir, "config.json")
			jsonData, err := json.Marshal(map[string]string{"schemaPath": tt.schemaPath})
			if err != nil {
				t.Fatalf("Failed to marshal config data: %v", err)
			}
			if err := os.WriteFile(configPath, jsonData, 0o644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			config, err := LoadConfigFile(configPath)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if want := tt.want(configDir); config.SchemaPath != want {
				t.Errorf("Expected SchemaPath to be '%s', got '%s'", want, config.SchemaPath)
			}
		})
	}
}

func TestLoadConfigFromBytes_SchemaPathDefault(t *testing.T) {
	config, err := LoadConfigFromBytes([]byte(`{}`))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config.SchemaPath != DefaultSchemaPath {
		t.Errorf("Expected SchemaPath to be '%s', got '%s'", DefaultSchemaPath, config.SchemaPath)
	}
}

func TestLoadConfigFile_ValidFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configData := map[string]interface{}{
		"ceUrl":  "http://ce.example.com",
		"pdpUrl": "http://pdp.example.com",
		"server": map[string]string{
			"port": "8080",
		},
	}

	jsonData, err := json.Marshal(configData)
	if err != nil {
		t.Fatalf("Failed to marshal config data: %v", err)
	}

	err = os.WriteFile(configPath, jsonData, 0o644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config == nil {
		t.Fatal("Expected config to be non-nil")
	}
	if config.CeURL != "http://ce.example.com" {
		t.Errorf("Expected CeURL to be 'http://ce.example.com', got '%s'", config.CeURL)
	}
	if config.PdpURL != "http://pdp.example.com" {
		t.Errorf("Expected PdpURL to be 'http://pdp.example.com', got '%s'", config.PdpURL)
	}
	if config.Server.Port != "8080" {
		t.Errorf("Expected Server.Port to be '8080', got '%s'", config.Server.Port)
	}
}

func TestLoadConfigFile_FileNotFound(t *testing.T) {
	config, err := LoadConfigFile("/nonexistent/path/config.json")

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if config != nil {
		t.Errorf("Expected config to be nil, got %v", config)
	}
	if !strings.Contains(err.Error(), "error reading config file") {
		t.Errorf("Expected error message to contain 'error reading config file', got '%s'", err.Error())
	}
}

func TestLoadConfigFile_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid.json")

	err := os.WriteFile(configPath, []byte(`{invalid json}`), 0o644)
	if err != nil {
		t.Fatalf("Failed to write invalid config file: %v", err)
	}

	config, err := LoadConfigFile(configPath)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if config != nil {
		t.Errorf("Expected config to be nil, got %v", config)
	}
	if !strings.Contains(err.Error(), "error unmarshaling config JSON") {
		t.Errorf("Expected error message to contain 'error unmarshaling config JSON', got '%s'", err.Error())
	}
}

func TestLoadConfig_DefaultPath(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Getenv("CONFIG_PATH")
	defer func() {
		if originalEnv != "" {
			os.Setenv("CONFIG_PATH", originalEnv)
		} else {
			os.Unsetenv("CONFIG_PATH")
		}
	}()

	os.Unsetenv("CONFIG_PATH")

	// LoadConfig tries to read ./config.json by default
	// This will fail since the file doesn't exist
	config, err := LoadConfig()

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if config != nil {
		t.Errorf("Expected config to be nil, got %v", config)
	}
	if !strings.Contains(err.Error(), "error reading config file") {
		t.Errorf("Expected error message to contain 'error reading config file', got '%s'", err.Error())
	}
}

func TestLoadConfig_CustomPath(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Getenv("CONFIG_PATH")
	defer func() {
		if originalEnv != "" {
			os.Setenv("CONFIG_PATH", originalEnv)
		} else {
			os.Unsetenv("CONFIG_PATH")
		}
	}()

	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "custom_config.json")

	configData := map[string]interface{}{
		"ceUrl": "http://ce.example.com",
	}

	jsonData, err := json.Marshal(configData)
	if err != nil {
		t.Fatalf("Failed to marshal config data: %v", err)
	}

	err = os.WriteFile(configPath, jsonData, 0o644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	os.Setenv("CONFIG_PATH", configPath)

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config == nil {
		t.Fatal("Expected config to be non-nil")
	}
	if config.CeURL != "http://ce.example.com" {
		t.Errorf("Expected CeURL to be 'http://ce.example.com', got '%s'", config.CeURL)
	}
}

func TestGetProviders(t *testing.T) {
	config := &Config{
		Providers: []*ProviderConfig{
			{
				ProviderURL: "http://provider1.example.com",
				SchemaID:    "schema1",
				Auth: &auth.AuthConfig{
					Type: "bearer",
				},
			},
			{
				ProviderURL: "http://provider2.example.com",
				SchemaID:    "schema2",
			},
		},
	}

	providers := config.GetProviders()

	if len(providers) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(providers))
	}
	if providers[0] == nil {
		t.Error("Expected providers[0] to be non-nil")
	}
	if providers[1] == nil {
		t.Error("Expected providers[1] to be non-nil")
	}
}

func TestGetProviders_EmptyProviders(t *testing.T) {
	config := &Config{
		Providers: []*ProviderConfig{},
	}

	providers := config.GetProviders()

	if providers == nil {
		t.Fatal("Expected providers to be non-nil")
	}
	if len(providers) != 0 {
		t.Errorf("Expected 0 providers, got %d", len(providers))
	}
}

func TestConfig_AllFieldsUnmarshal(t *testing.T) {
	jsonData := []byte(`{
		"ceUrl": "http://ce.example.com",
		"pdpUrl": "http://pdp.example.com",
		"providers": [
			{
				"providerUrl": "http://provider1.example.com",
				"schemaId": "schema1",
				"auth": {
					"type": "bearer",
					"token": "token123"
				}
			}
		],
		"argMappings": [],
		"server": {
			"port": "9000"
		},
		"log": {
			"level": "debug"
		},
		"services": {
			"pdp_url": "http://services-pdp.example.com"
		},
		"pdpConfig": {
			"clientUrl": "http://pdp-client.example.com"
		},
		"ceConfig": {
			"clientUrl": "http://ce-client.example.com"
		},
		"schema": "type Query { test: String }",
		"sdl": "type Mutation { test: String }",
		"argMapping": []
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if config.CeURL != "http://ce.example.com" {
		t.Errorf("Expected CeURL to be 'http://ce.example.com', got '%s'", config.CeURL)
	}
	if config.PdpURL != "http://pdp.example.com" {
		t.Errorf("Expected PdpURL to be 'http://pdp.example.com', got '%s'", config.PdpURL)
	}
	if len(config.Providers) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(config.Providers))
	}
	if config.Server.Port != "9000" {
		t.Errorf("Expected Server.Port to be '9000', got '%s'", config.Server.Port)
	}
	if config.Log.Level != "debug" {
		t.Errorf("Expected Log.Level to be 'debug', got '%s'", config.Log.Level)
	}
	if config.Services.PdpURL != "http://services-pdp.example.com" {
		t.Errorf("Expected Services.PdpURL to be 'http://services-pdp.example.com', got '%s'", config.Services.PdpURL)
	}
	if config.PdpConfig.ClientURL != "http://pdp-client.example.com" {
		t.Errorf("Expected PdpConfig.ClientURL to be 'http://pdp-client.example.com', got '%s'", config.PdpConfig.ClientURL)
	}
	if config.CeConfig.ClientURL != "http://ce-client.example.com" {
		t.Errorf("Expected CeConfig.ClientURL to be 'http://ce-client.example.com', got '%s'", config.CeConfig.ClientURL)
	}
	// The removed "schema" and "sdl" keys are still accepted, and ignored.
}

func TestProviderConfig_WithAuth(t *testing.T) {
	jsonData := []byte(`{
		"providers": [
			{
				"providerUrl": "http://secure.example.com",
				"schemaId": "secure-schema",
				"auth": {
					"type": "oauth2"
				}
			}
		]
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(config.Providers) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(config.Providers))
	}
	if config.Providers[0].Auth == nil {
		t.Fatal("Expected Auth to be non-nil")
	}
	if config.Providers[0].Auth.Type != auth.AuthType("oauth2") {
		t.Errorf("Expected Auth.Type to be 'oauth2', got '%s'", config.Providers[0].Auth.Type)
	}
}

func TestProviderConfig_WithoutAuth(t *testing.T) {
	jsonData := []byte(`{
		"providers": [
			{
				"providerUrl": "http://public.example.com",
				"schemaId": "public-schema"
			}
		]
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(config.Providers) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(config.Providers))
	}
	if config.Providers[0].Auth != nil {
		t.Errorf("Expected Auth to be nil, got %v", config.Providers[0].Auth)
	}
}

func TestConfig_MultipleProviders(t *testing.T) {
	jsonData := []byte(`{
		"providers": [
			{
				"providerUrl": "http://provider1.example.com",
				"schemaId": "schema1"
			},
			{
				"providerUrl": "http://provider2.example.com",
				"schemaId": "schema2"
			},
			{
				"providerUrl": "http://provider3.example.com",
				"schemaId": "schema3"
			}
		]
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(config.Providers) != 3 {
		t.Errorf("Expected 3 providers, got %d", len(config.Providers))
	}

	providers := config.GetProviders()
	if len(providers) != 3 {
		t.Errorf("Expected 3 providers from GetProviders, got %d", len(providers))
	}
}

func TestConfig_ArgMappings(t *testing.T) {
	jsonData := []byte(`{
		"argMappings": [
			{
				"sourceArg": "userId",
				"targetArg": "user_id"
			},
			{
				"sourceArg": "companyId",
				"targetArg": "company_id"
			}
		]
	}`)

	config, err := LoadConfigFromBytes(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(config.ArgMappings) != 2 {
		t.Errorf("Expected 2 argMappings, got %d", len(config.ArgMappings))
	}
	if config.ArgMapping == nil {
		t.Fatal("Expected ArgMapping to be non-nil")
	}
	if len(config.ArgMapping) != len(config.ArgMappings) {
		t.Error("Expected ArgMapping to equal ArgMappings")
	}
}
