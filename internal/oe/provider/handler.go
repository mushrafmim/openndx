package provider

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Handler is the main struct that holds all the provider handling information
type Handler struct {
	mu         sync.RWMutex
	Providers  []*Provider
	HttpClient *http.Client
}

// NewProviderHandler creates a new ProviderHandler with the given providers.
// Nil providers and providers without a schema ID are skipped; two providers
// with the same schema ID are an error, since a schema ID routes to one provider.
func NewProviderHandler(providers []*Provider) (*Handler, error) {
	providerMap := make([]*Provider, 0)
	seen := make(map[string]bool)

	// Create an http client with a 10 seconds timeout
	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: http.DefaultTransport,
	}

	for _, p := range providers {
		if p == nil || p.SchemaID == "" {
			continue
		}
		if seen[p.SchemaID] {
			return nil, fmt.Errorf("duplicate provider for schema %q", p.SchemaID)
		}
		seen[p.SchemaID] = true
		providerMap = append(providerMap, p)
		p.Client = httpClient
	}

	return &Handler{
		Providers:  providerMap,
		HttpClient: httpClient,
	}, nil
}

// GetProvider retrieves the provider that serves schemaID.
func (h *Handler) GetProvider(schemaID string) (*Provider, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var p *Provider
	exists := false
	for _, provider := range h.Providers {
		if provider.SchemaID == schemaID {
			p = provider
			exists = true
			break
		}
	}
	return p, exists
}

// AddProvider adds a new provider to the handler. It reports false, leaving the
// handler unchanged, when the provider is nil, has no schema ID, or a provider
// with the same schema ID is already registered.
func (h *Handler) AddProvider(provider *Provider) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if provider == nil || provider.SchemaID == "" {
		return false
	}

	for _, existing := range h.Providers {
		if existing.SchemaID == provider.SchemaID {
			return false
		}
	}

	h.Providers = append(h.Providers, provider)
	provider.Client = h.HttpClient
	return true
}
