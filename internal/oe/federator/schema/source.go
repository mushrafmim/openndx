// Package schema supplies the consumer schema the federator plans queries against.
package schema

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/source"
	"github.com/openndx/openndx-core/internal/oe/logger"
)

// Store returns the SDL of the schema activated through the schema management API.
type Store interface {
	// ActiveSchemaSDL returns the SDL of the active schema, or "" when no schema is active.
	ActiveSchemaSDL() (string, error)
}

// filePaths are tried in order when neither the store nor the config has a schema.
var filePaths = []string{
	"schema.graphql",
	"./schema.graphql",
	"../schema.graphql",
	"../../schema.graphql",
}

// Source returns the active consumer schema. It reads, in order, the schema
// store, the schema from config, and a schema.graphql file.
type Source struct {
	store     Store
	configSDL string
}

// NewSource returns a Source over store and configSDL. Either may be empty:
// a nil store or an empty configSDL is skipped.
func NewSource(store Store, configSDL string) *Source {
	return &Source{store: store, configSDL: configSDL}
}

// Active returns the parsed active schema. The schema is read again on every call.
func (s *Source) Active(_ context.Context) (*ast.Document, error) {
	if doc := s.fromStore(); doc != nil {
		return doc, nil
	}

	if s.configSDL != "" {
		doc, err := Parse(s.configSDL, "ConfigSchema")
		if err == nil {
			return doc, nil
		}
		logger.Log.Warn("Failed to get schema from config", "Error", err)
	}

	logger.Log.Info("No schema found in database or config, attempting to load schema.graphql file")
	return fromFile()
}

// fromStore returns the store's active schema, or nil when there is none or it cannot be read.
func (s *Source) fromStore() *ast.Document {
	if s.store == nil {
		logger.Log.Info("Schema store is nil, skipping database schema lookup")
		return nil
	}
	sdl, err := s.store.ActiveSchemaSDL()
	if err != nil {
		logger.Log.Warn("Failed to get active schema from database", "Error", err)
		return nil
	}
	if sdl == "" {
		return nil
	}
	doc, err := Parse(sdl, "ActiveSchema")
	if err != nil {
		logger.Log.Error("Failed to parse active schema from database", "Error", err)
		return nil
	}
	return doc
}

// fromFile loads the schema from the first schema.graphql found in filePaths.
func fromFile() (*ast.Document, error) {
	var errs []error
	for _, path := range filePaths {
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		doc, err := Parse(string(data), "SchemaFile")
		if err != nil {
			return nil, err
		}
		logger.Log.Info("Successfully loaded schema from schema.graphql file", "path", path)
		return doc, nil
	}
	return nil, fmt.Errorf("could not find schema.graphql file in any expected location: %w", errors.Join(errs...))
}

// Parse parses an SDL document; name identifies its origin in parse errors.
func Parse(sdl, name string) (*ast.Document, error) {
	src := source.NewSource(&source.Source{
		Body: []byte(sdl),
		Name: name,
	})
	return parser.Parse(parser.ParseParams{Source: src})
}
