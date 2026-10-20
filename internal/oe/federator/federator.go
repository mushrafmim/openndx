package federator

import (
	"context"
	stderrors "errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/source"
	"github.com/openndx/openndx-core/internal/monitoring"
	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/internals/errors"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
)

// Federator splits consumer queries across providers and merges their responses.
// Build one with New.
type Federator struct {
	deps Deps
	opts Options
}

type FederationServiceAST struct {
	ServiceKey string
	SchemaID   string
	QueryAst   *ast.Document
}

type federationServiceRequest struct {
	ServiceKey     string
	SchemaID       string
	GraphQLRequest graphql.Request
}

type ProviderResponse struct {
	ServiceKey string
	Response   graphql.Response `json:"response"`
}

type FederationResponse struct {
	ServiceKey string              `json:"ProviderKey"`
	Responses  []*ProviderResponse `json:"responses"`
}

// GetProviderResponse Returns the specific provider response by service key
func (f *FederationResponse) GetProviderResponse(providerKey string) *ProviderResponse {
	for _, resp := range f.Responses {
		if resp.ServiceKey == providerKey {
			return resp
		}
	}
	return nil
}

// createErrorResponse creates a GraphQL error response with the given message and optional extensions
func createErrorResponse(message string, extensions map[string]interface{}) graphql.Response {
	errorMap := map[string]interface{}{
		"message": message,
	}
	if extensions != nil {
		errorMap["extensions"] = extensions
	}
	return graphql.Response{
		Data: nil,
		Errors: []interface{}{
			errorMap,
		},
	}
}

// createErrorResponseWithCode creates a GraphQL error response with a message and error code
func createErrorResponseWithCode(message string, code string) graphql.Response {
	return createErrorResponse(message, map[string]interface{}{
		"code": code,
	})
}

// errorResponseFromError wraps err in a GraphQL error response, keeping it as-is
// when it is already a *graphql.JSONError.
func errorResponseFromError(err error) graphql.Response {
	var jsonErr *graphql.JSONError
	if !stderrors.As(err, &jsonErr) {
		jsonErr = &graphql.JSONError{Message: err.Error()}
	}
	return graphql.Response{
		Data:   nil,
		Errors: []interface{}{jsonErr},
	}
}

// FederateQuery takes a raw GraphQL query, splits it into sub-queries for each service,
// sends them to the respective providers, and merges the responses.
func (f *Federator) FederateQuery(ctx context.Context, request graphql.Request, consumerInfo *auth.ConsumerAssertion) graphql.Response {
	// Ensure traceID is in context (should already be set by monitoring.TraceIDMiddleware, but ensure it)
	traceID := monitoring.GetTraceIDFromContext(ctx)
	if traceID == "" {
		traceID = uuid.New().String()
		ctx = monitoring.WithTraceID(ctx, traceID)
	}

	appID := consumerInfo.ApplicationID
	ctx = f.deps.Auditor.RequestReceived(ctx, appID, request.Query)

	query, invalid := parseQuery(request.Query)
	if invalid != nil {
		logger.Log.Info("Failed to parse query", "errors", invalid.Errors)
		return *invalid
	}

	compiled, err := f.deps.Schemas.Active(ctx)
	if err != nil {
		logger.Log.Error("Failed to load the active schema", "Error", err)
		return graphql.Response{
			Data: nil,
			Errors: []interface{}{
				&graphql.JSONError{
					Message: "No active schema found.",
				},
			},
		}
	}

	if invalid := validateQuery(compiled.Schema, query); invalid != nil {
		logger.Log.Info("Query failed validation against the schema", "errors", invalid.Errors)
		return *invalid
	}

	// The mapper, builder and accumulator still work on graphql-go's AST, so the
	// validated query is parsed again (#640, step 6 removes this).
	schema := compiled.Document
	doc, err := parser.Parse(parser.ParseParams{Source: source.NewSource(&source.Source{
		Body: []byte(request.Query),
		Name: "Query",
	})})
	if err != nil {
		logger.Log.Error("Failed to parse query", "Error", err)
		return createErrorResponseWithCode(fmt.Sprintf("Failed to parse query: %v", err), errors.CodeBadRequest)
	}

	// Collect the directives from the query
	schemaCollection, err := ProviderSchemaCollector(schema, doc)
	if err != nil {
		logger.Log.Error("Failed to collect provider schema", "Error", err)
		return errorResponseFromError(err)
	}

	var fields []ProviderLevelFieldRecord
	if schemaCollection.ProviderFieldMap != nil {
		fields = *schemaCollection.ProviderFieldMap
	}

	requiredArguments := FindRequiredArguments(schemaCollection.ProviderFieldMap, f.opts.ArgMapping)

	extractedArgs := ExtractRequiredArguments(requiredArguments, schemaCollection.Arguments)

	// check whether there are variables in the request
	if request.Variables != nil {
		// if there are variables, replace the argument values with the variable values
		PushVariablesFromVariableDefinition(request, extractedArgs, schemaCollection.VariableDefinitions)
	}

	ctx, denied := f.authorizeAccess(ctx, appID, fields, extractedArgs)
	if denied != nil {
		return *denied
	}

	splitRequests, err := QueryBuilder(schemaCollection.ProviderFieldMap, extractedArgs)
	if err != nil {
		logger.Log.Error("Failed to build queries", "Error", err)
		return errorResponseFromError(err)
	}

	if len(splitRequests) == 0 {
		logger.Log.Info("No valid service queries found in the request")
		return createErrorResponse("No valid service queries found in the request", nil)
	}

	responses := f.performFederation(ctx, appID, fields, splitRequests)

	// Build schema info map for array-aware processing
	schemaInfoMap, err := BuildSchemaInfoMap(schema, doc)
	if err != nil {
		logger.Log.Error("Failed to build schema info map", "Error", err)
	}

	// Transform the federated responses back to the original query structure using array-aware processing
	return AccumulateResponseWithSchemaInfo(doc, responses, schemaInfoMap)
}

// performFederation sends each sub-query to its provider concurrently and collects the
// responses. fields are the provider fields the query reads, used for auditing.
func (f *Federator) performFederation(ctx context.Context, appID string, fields []ProviderLevelFieldRecord, requests []*federationServiceRequest) *FederationResponse {
	federationResponse := &FederationResponse{
		Responses: make([]*ProviderResponse, 0, len(requests)),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex // to safely append to federationResponse.Responses

	for _, request := range requests {
		client, exists := f.deps.Providers.Get(request.ServiceKey, request.SchemaID)
		if !exists {
			logger.Log.Info("Provider not found", "Provider Key", request.ServiceKey)
			continue
		}

		fetch := ProviderFetch{
			ApplicationID:   appID,
			ServiceKey:      request.ServiceKey,
			SchemaID:        request.SchemaID,
			Query:           request.GraphQLRequest.Query,
			RequestedFields: requestedFields(fields, request.ServiceKey, request.SchemaID),
		}

		wg.Add(1)
		go func(req *federationServiceRequest, client ProviderClient) {
			defer wg.Done()

			response, err := client.Execute(ctx, req.GraphQLRequest)
			if err != nil {
				logger.Log.Info("Request failed to the Provider", "Provider Key", req.ServiceKey, "Error", err)
				f.deps.Auditor.ProviderFetch(ctx, fetch, nil, err)
				return
			}
			f.deps.Auditor.ProviderFetch(ctx, fetch, &response, nil)

			mu.Lock()
			federationResponse.Responses = append(federationResponse.Responses, &ProviderResponse{
				ServiceKey: req.ServiceKey,
				Response:   response,
			})
			mu.Unlock()
		}(request, client)
	}

	wg.Wait()
	return federationResponse
}

// requestedFields returns the paths of the fields served by the provider serviceKey for schemaID.
func requestedFields(fields []ProviderLevelFieldRecord, serviceKey, schemaID string) []string {
	paths := make([]string, 0)
	for _, field := range fields {
		if field.ServiceKey == serviceKey && field.SchemaId == schemaID {
			paths = append(paths, field.FieldPath)
		}
	}
	return paths
}
