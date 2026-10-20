package federator

import (
	"context"
	"testing"

	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/source"
	"github.com/openndx/openndx-core/internal/oe/configs"
	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
	"github.com/stretchr/testify/require"
)

// newTestFederator builds a Federator from cfg and schemas with the real PDP,
// consent and provider clients, the way cmd/oe/main.go does.
func newTestFederator(t *testing.T, cfg *configs.Config, schemas SchemaSource) *Federator {
	t.Helper()

	deps := Deps{
		Providers: ProviderLookup(provider.NewProviderHandler(cfg.GetProviders()).GetProvider),
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

const defaultTestSchemaSDL = `
directive @sourceInfo(
	providerKey: String!
	schemaId: String
	providerField: String!
) on FIELD_DEFINITION

type Query {
	personInfo(nic: String!): PersonInfo
	personInfos(nics: [String!]!): [PersonInfo]
}

type PersonInfo {
	fullName: String @sourceInfo(providerKey: "drp", schemaId: "drp-schema-v1", providerField: "person.fullName")
	name: String @sourceInfo(providerKey: "rgd", schemaId: "rgd-schema-v1", providerField: "getPersonInfo.name")
	address: String @sourceInfo(providerKey: "drp", schemaId: "drp-schema-v1", providerField: "person.permanentAddress")
	ownedVehicles: [VehicleInfo] @sourceInfo(providerKey: "dmt", schemaId: "dmt-schema-v1", providerField: "vehicle.getVehicleInfos.data")
	class: [VehicleClass] @sourceInfo(providerKey: "dmt", schemaId: "dmt-schema-v1", providerField: "vehicle.getVehicleInfos.classes")
}

type VehicleInfo {
	regNo: String @sourceInfo(providerKey: "dmt", schemaId: "dmt-schema-v1", providerField: "vehicle.getVehicleInfos.data.registrationNumber")
	make: String @sourceInfo(providerKey: "dmt", schemaId: "dmt-schema-v1", providerField: "vehicle.getVehicleInfos.data.make")
	model: String @sourceInfo(providerKey: "dmt", schemaId: "dmt-schema-v1", providerField: "vehicle.getVehicleInfos.data.model")
}

type VehicleClass {
	className: String @sourceInfo(providerKey: "dmt", schemaId: "dmt-schema-v1", providerField: "vehicle.getVehicleInfos.classes.className")
}
`

func mustParseDocument(t *testing.T, input string) *ast.Document {
	t.Helper()

	src := source.NewSource(&source.Source{
		Body: []byte(input),
		Name: "TestDoc",
	})

	doc, err := parser.Parse(parser.ParseParams{Source: src})
	require.NoError(t, err)
	return doc
}

func ParseTestQuery(t *testing.T, query string) *ast.Document {
	return mustParseDocument(t, query)
}

func ParseSchemaDoc(t *testing.T, sdl string) *ast.Document {
	return mustParseDocument(t, sdl)
}

func ParseQueryDoc(t *testing.T, query string) *ast.Document {
	return mustParseDocument(t, query)
}

func CreateTestSchema(t *testing.T) *ast.Document {
	return ParseSchemaDoc(t, defaultTestSchemaSDL)
}
