package federator

import (
	"context"
	stderrors "errors"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/monitoring"
	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/federator/operation"
	"github.com/openndx/openndx-core/internal/oe/internals/errors"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Federator splits consumer queries across providers and merges their responses.
// Build one with New.
type Federator struct {
	deps Deps
	opts Options
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

	op, err := operation.Build(compiled, query, request.OperationName, request.Variables)
	if err != nil {
		logger.Log.Info("Failed to normalize query", "Error", err)
		return operationErrorResponse(err)
	}

	dataOwnerID, denied := f.dataOwnerID(op)
	if denied != nil {
		return *denied
	}

	queryPlan, err := buildPlan(compiled.Schema, op, f.opts.ArgMapping)
	if err != nil {
		logger.Log.Info("Failed to plan query", "Error", err)
		return createErrorResponseWithCode(err.Error(), errors.CodeBadRequest)
	}

	ctx, denied = f.authorizeAccess(ctx, appID, queryPlan.requiredFields(), dataOwnerID)
	if denied != nil {
		return *denied
	}

	if len(queryPlan.fetches) == 0 {
		logger.Log.Info("No valid service queries found in the request")
		return createErrorResponse("No valid service queries found in the request", nil)
	}

	results := f.execute(ctx, appID, queryPlan.fetches)
	return merge(op, queryPlan, results)
}

// operationErrorResponse reports an error from normalizing a validated query.
func operationErrorResponse(err error) graphql.Response {
	var gqlErr *gqlerror.Error
	switch {
	case stderrors.As(err, &gqlErr):
		return *invalidQueryResponse(gqlerror.List{gqlErr})
	case stderrors.Is(err, operation.ErrUnsupportedOperation):
		return createErrorResponse("Only query operation is supported", nil)
	default:
		return createErrorResponseWithCode(err.Error(), errors.CodeBadRequest)
	}
}
