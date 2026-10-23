package federator

import (
	"context"
	"os"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/configs"
	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// The federator and its clients log through the package-level logger.
	logger.Init()
	os.Exit(m.Run())
}

// newTestFederator builds a Federator from cfg and schemas with the real PDP,
// consent and provider clients, the way cmd/oe/main.go does.
func newTestFederator(t *testing.T, cfg *configs.Config, schemas SchemaSource) *Federator {
	t.Helper()

	providers, err := provider.NewProviderHandler(cfg.GetProviders())
	require.NoError(t, err)
	deps := Deps{
		Providers: ProviderLookup(providers.GetProvider),
		Schemas:   schemas,
	}
	if cfg.PdpConfig.ClientURL != "" {
		deps.Policy = policy.NewPdpClient(cfg.PdpConfig.ClientURL)
	}
	if cfg.CeConfig.ClientURL != "" {
		deps.Consent = consent.NewCEServiceClient(cfg.CeConfig.ClientURL)
	}
	f, err := New(deps, OptionsFromConfig(cfg))
	require.NoError(t, err)
	return f
}

// staticSchema is a SchemaSource that always serves the same SDL.
type staticSchema string

func (s staticSchema) Active(context.Context) (*schema.CompiledSchema, error) {
	return schema.Compile(string(s))
}
