package federator

import (
	"context"
	"fmt"
	"sync"

	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
)

// fetchResult is the outcome of one fetch: the provider's response, or the error
// that stopped it.
type fetchResult struct {
	response graphql.Response
	err      error
}

// execute runs fetches concurrently and returns their results when all have
// finished. Each provider call is audited.
func (f *Federator) execute(ctx context.Context, appID string, fetches []*fetch) map[*fetch]fetchResult {
	results := make(map[*fetch]fetchResult, len(fetches))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, ft := range fetches {
		client, ok := f.deps.Providers.Get(ft.providerKey, ft.schemaID)
		if !ok {
			logger.Log.Info("Provider not found", "Provider Key", ft.providerKey, "Schema ID", ft.schemaID)
			results[ft] = fetchResult{err: fmt.Errorf("provider %s with schema %s is not configured", ft.providerKey, ft.schemaID)}
			continue
		}

		wg.Add(1)
		go func(ft *fetch, client ProviderClient) {
			defer wg.Done()

			audit := ProviderFetch{
				ApplicationID:   appID,
				ServiceKey:      ft.providerKey,
				SchemaID:        ft.schemaID,
				Query:           ft.query,
				RequestedFields: ft.fields,
			}
			response, err := client.Execute(ctx, graphql.Request{Query: ft.query})
			if err != nil {
				logger.Log.Info("Request failed to the Provider", "Provider Key", ft.providerKey, "Error", err)
				f.deps.Auditor.ProviderFetch(ctx, audit, nil, err)
			} else {
				f.deps.Auditor.ProviderFetch(ctx, audit, &response, nil)
			}

			mu.Lock()
			results[ft] = fetchResult{response: response, err: err}
			mu.Unlock()
		}(ft, client)
	}

	wg.Wait()
	return results
}
