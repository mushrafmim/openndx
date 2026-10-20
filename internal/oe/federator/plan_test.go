package federator

import (
	"strings"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/federator/operation"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

const planTestSchema = `
enum Lang { EN SI }

type Query {
	personInfo(nic: String!, lang: Lang, year: Int): PersonInfo
	required(nic: String!): Required!
}

type PersonInfo {
	fullName: String @sourceInfo(providerKey: "drp", schemaId: "drp-v1", providerField: "person.fullName")
	tags: [String] @sourceInfo(providerKey: "drp", schemaId: "drp-v1", providerField: "person.tags")
	ownedVehicles: [Vehicle!] @sourceInfo(providerKey: "dmt", schemaId: "dmt-v1", providerField: "vehicle.list.data")
	untracked: String
	contacts: [Contact]
}

type Contact {
	phone: String @sourceInfo(providerKey: "drp", schemaId: "drp-v1", providerField: "person.phone")
}

type Vehicle {
	regNo: String! @sourceInfo(providerKey: "dmt", schemaId: "dmt-v1", providerField: "vehicle.list.data.registrationNumber")
	make: String @sourceInfo(providerKey: "dmt", schemaId: "dmt-v1", providerField: "make")
	owner: Owner
}

type Owner {
	name: String @sourceInfo(providerKey: "dmt", schemaId: "dmt-v1", providerField: "vehicle.list.data.owner.name")
}

type Required {
	value: String! @sourceInfo(providerKey: "drp", schemaId: "drp-v1", providerField: "required.value")
	optional: String @sourceInfo(providerKey: "drp", schemaId: "drp-v1", providerField: "required.optional")
}
`

var planTestMappings = []*graphql.ArgMapping{
	{ProviderKey: "drp", SchemaID: "drp-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "person", TargetArgName: "nic"},
	{ProviderKey: "drp", SchemaID: "drp-v1", SourceArgPath: "personInfo-lang", TargetArgPath: "person", TargetArgName: "lang"},
	{ProviderKey: "drp", SchemaID: "drp-v1", SourceArgPath: "personInfo-year", TargetArgPath: "person", TargetArgName: "year"},
	{ProviderKey: "dmt", SchemaID: "dmt-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "vehicle.list", TargetArgName: "ownerNic"},
	{ProviderKey: "drp", SchemaID: "drp-v1", SourceArgPath: "required-nic", TargetArgPath: "required", TargetArgName: "nic"},
}

// planFor normalizes and plans query against planTestSchema.
func planFor(t *testing.T, query string, variables map[string]any) (*operation.Operation, *plan) {
	t.Helper()
	compiled, err := schema.Compile(planTestSchema)
	require.NoError(t, err)
	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	require.NoError(t, err)
	require.Empty(t, validator.ValidateWithRules(compiled.Schema, doc, nil), "the test query must be valid")
	op, err := operation.Build(compiled, doc, "", variables)
	require.NoError(t, err)
	p, err := buildPlan(compiled.Schema, op, planTestMappings)
	require.NoError(t, err)
	return op, p
}

// compact joins a printed query onto one line.
func compact(query string) string {
	return strings.Join(strings.Fields(query), " ")
}

func TestBuildPlan(t *testing.T) {
	t.Run("builds one operation per provider with arguments only at their target path", func(t *testing.T) {
		_, p := planFor(t, `{ personInfo(nic: "1", lang: SI) { fullName tags ownedVehicles { regNo make owner { name } } untracked __typename } }`, nil)

		require.Len(t, p.fetches, 2)
		drp, dmt := p.fetches[0], p.fetches[1]
		assert.Equal(t, "drp", drp.providerKey)
		assert.Equal(t, `query Querydrp { person(lang: SI, nic: "1") { fullName tags } }`, compact(drp.query))
		assert.Equal(t, []string{"person.fullName", "person.tags"}, drp.fields)

		assert.Equal(t, "dmt", dmt.providerKey)
		assert.Equal(t, `query Querydmt { vehicle { list(ownerNic: "1") { data { registrationNumber make owner { name } } } } }`, compact(dmt.query),
			"a list item's field declared outside the list is read by its last segment")
		assert.Equal(t, []string{"vehicle.list.data.registrationNumber", "vehicle.list.data.make", "vehicle.list.data.owner.name"}, dmt.fields)

		assert.Equal(t, []policy.RequiredField{
			{SchemaID: "drp-v1", FieldName: "person.fullName"},
			{SchemaID: "drp-v1", FieldName: "person.tags"},
			{SchemaID: "dmt-v1", FieldName: "vehicle.list.data.registrationNumber"},
			{SchemaID: "dmt-v1", FieldName: "vehicle.list.data.make"},
			{SchemaID: "dmt-v1", FieldName: "vehicle.list.data.owner.name"},
		}, p.requiredFields(), "only leaves are authorized, not the list container")
	})

	t.Run("shares a fetch between aliases with the same arguments", func(t *testing.T) {
		_, p := planFor(t, `{ a: personInfo(nic: "1") { fullName } b: personInfo(nic: "1") { name: fullName tags } }`, nil)

		require.Len(t, p.fetches, 1)
		assert.Equal(t, `query Querydrp { person(nic: "1") { fullName tags } }`, compact(p.fetches[0].query))
		assert.Equal(t, []policy.RequiredField{
			{SchemaID: "drp-v1", FieldName: "person.fullName"},
			{SchemaID: "drp-v1", FieldName: "person.tags"},
		}, p.requiredFields(), "a provider field read twice is authorized once")
	})

	t.Run("splits fetches when aliases map different argument values", func(t *testing.T) {
		_, p := planFor(t, `{ a: personInfo(nic: "1", lang: EN) { fullName } b: personInfo(nic: "1", lang: SI) { fullName } }`, nil)

		require.Len(t, p.fetches, 2)
		assert.Equal(t, `query Querydrp { person(lang: EN, nic: "1") { fullName } }`, compact(p.fetches[0].query))
		assert.Equal(t, `query Querydrp { person(lang: SI, nic: "1") { fullName } }`, compact(p.fetches[1].query))
	})

	t.Run("prints variables and defaults by their consumer argument type", func(t *testing.T) {
		_, p := planFor(t, `query ($lang: Lang = SI, $year: Int) { personInfo(nic: "1", lang: $lang, year: $year) { fullName } }`,
			map[string]any{"year": float64(2024)})

		require.Len(t, p.fetches, 1)
		assert.Equal(t, `query Querydrp { person(lang: SI, nic: "1", year: 2024) { fullName } }`, compact(p.fetches[0].query),
			"an enum prints as an enum, and a whole JSON number for an Int as an Int")
	})

	t.Run("plans nothing for __typename and fields without @sourceInfo", func(t *testing.T) {
		_, p := planFor(t, `{ __typename personInfo(nic: "1") { untracked __typename } }`, nil)

		assert.Empty(t, p.fetches)
	})

	t.Run("rejects a list field with a selection but no @sourceInfo", func(t *testing.T) {
		compiled, err := schema.Compile(planTestSchema)
		require.NoError(t, err)
		doc, err := parser.ParseQuery(&ast.Source{Input: `{ personInfo(nic: "1") { contacts { phone } } }`})
		require.NoError(t, err)
		require.Empty(t, validator.ValidateWithRules(compiled.Schema, doc, nil))
		op, err := operation.Build(compiled, doc, "", nil)
		require.NoError(t, err)

		_, err = buildPlan(compiled.Schema, op, planTestMappings)
		assert.EqualError(t, err, "list field personInfo.contacts has no @sourceInfo, so its items can't be read from a provider")
	})

	t.Run("rejects introspection fields", func(t *testing.T) {
		compiled, err := schema.Compile(planTestSchema)
		require.NoError(t, err)
		doc, err := parser.ParseQuery(&ast.Source{Input: `{ __schema { queryType { name } } }`})
		require.NoError(t, err)
		require.Empty(t, validator.ValidateWithRules(compiled.Schema, doc, nil))
		op, err := operation.Build(compiled, doc, "", nil)
		require.NoError(t, err)

		_, err = buildPlan(compiled.Schema, op, planTestMappings)
		assert.EqualError(t, err, "introspection field __schema is not supported")
	})
}

func TestAstValue(t *testing.T) {
	compiled, err := schema.Compile(planTestSchema)
	require.NoError(t, err)
	stringType := &ast.Type{NamedType: "String"}
	intType := &ast.Type{NamedType: "Int"}

	for name, tc := range map[string]struct {
		value any
		typ   *ast.Type
		want  string
	}{
		"string":           {value: `say "hi"`, typ: stringType, want: `"say \"hi\""`},
		"enum":             {value: "SI", typ: &ast.Type{NamedType: "Lang"}, want: `SI`},
		"null":             {value: nil, typ: stringType, want: `null`},
		"boolean":          {value: true, want: `true`},
		"int64":            {value: int64(7), typ: intType, want: `7`},
		"whole float Int":  {value: float64(7), typ: intType, want: `7`},
		"float":            {value: 7.5, want: `7.5`},
		"list":             {value: []any{"a", "b"}, typ: &ast.Type{Elem: stringType}, want: `["a","b"]`},
		"object keys sort": {value: map[string]any{"b": int64(2), "a": "x"}, want: `{a:"x",b:2}`},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := astValue(tc.value, tc.typ, compiled.Schema)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.String())
		})
	}

	_, err = astValue(struct{}{}, nil, compiled.Schema)
	assert.EqualError(t, err, "unsupported value of type struct {}")
}
