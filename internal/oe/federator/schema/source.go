// Package schema supplies the consumer schema the federator plans queries against.
package schema

import (
	"context"
	"fmt"
	"os"

	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/source"
)

// DefaultPath is where the consumer schema is read from when SCHEMA_PATH is unset.
const DefaultPath = "schema.graphql"

// Source serves the consumer schema. It is loaded once and doesn't change while
// the OE runs: to change the schema, deploy a new schema file and restart.
type Source struct {
	doc *ast.Document
}

// Load reads and parses the consumer schema file at path.
func Load(path string) (*Source, error) {
	sdl, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read schema file: %w", err)
	}
	src, err := FromSDL(string(sdl))
	if err != nil {
		return nil, fmt.Errorf("parse schema file %s: %w", path, err)
	}
	return src, nil
}

// FromSDL parses sdl as the consumer schema.
func FromSDL(sdl string) (*Source, error) {
	doc, err := parser.Parse(parser.ParseParams{Source: source.NewSource(&source.Source{
		Body: []byte(sdl),
		Name: "Schema",
	})})
	if err != nil {
		return nil, err
	}
	return &Source{doc: doc}, nil
}

// Active returns the consumer schema. The document is shared by all requests and
// must not be modified.
func (s *Source) Active(_ context.Context) (*ast.Document, error) {
	return s.doc, nil
}
