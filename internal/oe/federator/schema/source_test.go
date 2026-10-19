package schema

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/graphql-go/graphql/language/ast"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logger.Init()
}

type fakeStore struct {
	sdl string
	err error
}

func (s fakeStore) ActiveSchemaSDL() (string, error) { return s.sdl, s.err }

// queryField returns the name of the first field on the schema's Query type.
func queryField(t *testing.T, doc *ast.Document) string {
	t.Helper()
	for _, def := range doc.Definitions {
		if obj, ok := def.(*ast.ObjectDefinition); ok && obj.Name.Value == "Query" {
			return obj.Fields[0].Name.Value
		}
	}
	t.Fatal("schema has no Query type")
	return ""
}

// inDir runs the test from dir so schema.graphql lookups start there.
func inDir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

func TestSource_Active(t *testing.T) {
	const storeSDL = `type Query { fromStore: String }`
	const configSDL = `type Query { fromConfig: String }`
	const fileSDL = `type Query { fromFile: String }`

	t.Run("prefers the store's active schema", func(t *testing.T) {
		doc, err := NewSource(fakeStore{sdl: storeSDL}, configSDL).Active(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "fromStore", queryField(t, doc))
	})

	for name, store := range map[string]Store{
		"no store":                   nil,
		"store has no active schema": fakeStore{},
		"store fails":                fakeStore{err: errors.New("database unavailable")},
		"store schema is invalid":    fakeStore{sdl: "type Query {"},
	} {
		t.Run("falls back to config when "+name, func(t *testing.T) {
			doc, err := NewSource(store, configSDL).Active(context.Background())
			require.NoError(t, err)
			assert.Equal(t, "fromConfig", queryField(t, doc))
		})
	}

	t.Run("falls back to schema.graphql", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "schema.graphql"), []byte(fileSDL), 0o600))
		inDir(t, dir)

		doc, err := NewSource(nil, "type Query {").Active(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "fromFile", queryField(t, doc))
	})

	t.Run("fails when no schema is available", func(t *testing.T) {
		// Nested so the ../ lookups stay inside the temporary directory.
		dir := filepath.Join(t.TempDir(), "a", "b", "c")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		inDir(t, dir)

		_, err := NewSource(nil, "").Active(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not find schema.graphql")
	})
}
