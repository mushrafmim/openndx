package federator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/openndx/openndx-core/internal/oe/federator/operation"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
)

// plan is the set of provider operations that answer a normalized consumer
// operation, and how each consumer field maps back to them.
type plan struct {
	// fetches run in parallel: every fetch depends only on the consumer's arguments.
	fetches []*fetch
	// sources maps each consumer field that reads provider data directly (a leaf,
	// or a list whose items are read relative to it) to the fetch that serves it.
	sources map[*operation.Field]*fetch
}

// fetch is one provider operation.
type fetch struct {
	providerKey string
	schemaID    string
	// query is the printed provider operation.
	query string
	// fields are the provider field paths the operation reads, in query order.
	fields []string
	// consumerFields are the consumer fields served directly by this fetch, used
	// to place provider errors in the consumer response.
	consumerFields []*operation.Field

	key  string
	root *selectionNode
	args []providerArgument
}

// providerArgument is a mapped argument attached to the provider field at path.
type providerArgument struct {
	path  string
	name  string
	value *ast.Value
}

// elementScope is the provider list whose items the fields inside a consumer list
// field are read from.
type elementScope struct {
	fetch *fetch
	// declared is the list's providerField: its items' fields are declared
	// relative to it, or by their last path segment.
	declared string
	// requested is the provider path the list is requested at.
	requested string
}

// buildPlan plans the provider operations for op. Each argument mapping is
// attached only at its targetArgPath, with the value bound from the consumer
// field that carries it or one of its ancestors.
func buildPlan(compiled *ast.Schema, op *operation.Operation, mappings []*graphql.ArgMapping) (*plan, error) {
	p := &planner{
		schema:   compiled,
		bindings: op.BindArguments(mappings),
		plan:     &plan{sources: map[*operation.Field]*fetch{}},
		byKey:    map[string]*fetch{},
	}
	if err := p.fields(op.Fields, nil); err != nil {
		return nil, err
	}
	for _, f := range p.plan.fetches {
		f.query = f.print()
	}
	return p.plan, nil
}

// requiredFields returns the provider fields the PDP must authorize: every leaf
// the plan reads, once.
func (p *plan) requiredFields() []policy.RequiredField {
	seen := map[policy.RequiredField]bool{}
	var fields []policy.RequiredField
	for _, f := range p.fetches {
		for _, path := range f.fields {
			field := policy.RequiredField{SchemaID: f.schemaID, FieldName: path}
			if !seen[field] {
				seen[field] = true
				fields = append(fields, field)
			}
		}
	}
	return fields
}

type planner struct {
	schema   *ast.Schema
	bindings []operation.ArgumentBinding
	plan     *plan
	byKey    map[string]*fetch
}

// fields plans fields. scope is nil outside a provider list.
func (p *planner) fields(fields []*operation.Field, scope *elementScope) error {
	for _, field := range fields {
		if err := p.field(field, scope); err != nil {
			return err
		}
	}
	return nil
}

func (p *planner) field(field *operation.Field, scope *elementScope) error {
	if field.Name == "__typename" {
		return nil
	}
	if strings.HasPrefix(field.Name, "__") {
		return fmt.Errorf("introspection field %s is not supported", field.Name)
	}

	info := field.SourceInfo
	isList := field.Definition.Type.Elem != nil
	switch {
	case scope != nil && info != nil && (isList || len(field.Selections) == 0):
		requested := scope.requested + "." + relativePath(info.ProviderField, scope.declared)
		if isList && len(field.Selections) > 0 {
			return p.fields(field.Selections, &elementScope{fetch: scope.fetch, declared: info.ProviderField, requested: requested})
		}
		scope.fetch.addField(requested)
		return nil

	case scope == nil && info != nil && (isList || len(field.Selections) == 0):
		f, err := p.fetchFor(field)
		if err != nil {
			return err
		}
		p.plan.sources[field] = f
		f.consumerFields = append(f.consumerFields, field)
		if isList && len(field.Selections) > 0 {
			return p.fields(field.Selections, &elementScope{fetch: f, declared: info.ProviderField, requested: info.ProviderField})
		}
		f.addField(info.ProviderField)
		return nil

	default:
		// An object field without @sourceInfo, or one whose @sourceInfo only
		// documents its provider object: its fields carry their own sources.
		return p.fields(field.Selections, scope)
	}
}

// fetchFor returns the fetch that serves field: the one for its provider and
// schema with the same mapped arguments, created on first use.
func (p *planner) fetchFor(field *operation.Field) (*fetch, error) {
	info := field.SourceInfo
	args, err := p.arguments(field)
	if err != nil {
		return nil, err
	}

	var key strings.Builder
	key.WriteString(info.ProviderKey + "\x00" + info.SchemaID)
	for _, arg := range args {
		key.WriteString("\x00" + arg.path + "\x00" + arg.name + "\x00" + arg.value.String())
	}
	if f, ok := p.byKey[key.String()]; ok {
		return f, nil
	}

	f := &fetch{
		providerKey: info.ProviderKey,
		schemaID:    info.SchemaID,
		key:         key.String(),
		root:        &selectionNode{},
		args:        args,
	}
	p.byKey[f.key] = f
	p.plan.fetches = append(p.plan.fetches, f)
	return f, nil
}

// arguments returns the mapped arguments for the provider field that serves
// field: the bindings for its provider and schema, carried by field or an
// ancestor, whose targetArgPath leads to the provider field. The first binding
// for each target path and argument wins; the result is sorted.
func (p *planner) arguments(field *operation.Field) ([]providerArgument, error) {
	info := field.SourceInfo
	seen := map[[2]string]bool{}
	var args []providerArgument
	for _, binding := range p.bindings {
		mapping := binding.Mapping
		if mapping.ProviderKey != info.ProviderKey || mapping.SchemaID != info.SchemaID ||
			!isPathPrefix(info.ProviderField, mapping.TargetArgPath) ||
			!hasPrefix(field.Path, binding.Field.Path) {
			continue
		}
		id := [2]string{mapping.TargetArgPath, mapping.TargetArgName}
		if seen[id] {
			continue
		}
		seen[id] = true

		segments := strings.Split(mapping.SourceArgPath, "-")
		argDef := binding.Field.Definition.Arguments.ForName(segments[len(segments)-1])
		var argType *ast.Type
		if argDef != nil {
			argType = argDef.Type
		}
		value, err := astValue(binding.Value, argType, p.schema)
		if err != nil {
			return nil, fmt.Errorf("argument %s of %s: %w", mapping.TargetArgName, mapping.TargetArgPath, err)
		}
		args = append(args, providerArgument{path: mapping.TargetArgPath, name: mapping.TargetArgName, value: value})
	}
	sort.Slice(args, func(i, j int) bool {
		if args[i].path != args[j].path {
			return args[i].path < args[j].path
		}
		return args[i].name < args[j].name
	})
	return args, nil
}

// addField adds the provider field at path to the operation, once.
func (f *fetch) addField(path string) {
	if slices.Contains(f.fields, path) {
		return
	}
	f.fields = append(f.fields, path)
	node := f.root
	for _, name := range strings.Split(path, ".") {
		node = node.child(name)
	}
}

// print returns the provider operation, named "Query" + the provider key.
func (f *fetch) print() string {
	for _, arg := range f.args {
		node := f.root
		for _, name := range strings.Split(arg.path, ".") {
			node = node.child(name)
		}
		node.arguments = append(node.arguments, &ast.Argument{Name: arg.name, Value: arg.value})
	}

	doc := &ast.QueryDocument{Operations: ast.OperationList{{
		Operation:    ast.Query,
		Name:         "Query" + f.providerKey,
		SelectionSet: f.root.selectionSet(),
	}}}
	var buf bytes.Buffer
	formatter.NewFormatter(&buf, formatter.WithIndent("  ")).FormatQueryDocument(doc)
	return strings.TrimSpace(buf.String())
}

// selectionNode is a provider field in a fetch's selection tree.
type selectionNode struct {
	name      string
	arguments ast.ArgumentList
	children  []*selectionNode
}

func (n *selectionNode) child(name string) *selectionNode {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	c := &selectionNode{name: name}
	n.children = append(n.children, c)
	return c
}

func (n *selectionNode) selectionSet() ast.SelectionSet {
	if len(n.children) == 0 {
		return nil
	}
	set := make(ast.SelectionSet, 0, len(n.children))
	for _, c := range n.children {
		set = append(set, &ast.Field{Name: c.name, Arguments: c.arguments, SelectionSet: c.selectionSet()})
	}
	return set
}

// relativePath returns the path of a list item's field, given its declared
// providerField and the list's: the part after the list's path, or, for fields
// declared outside it, the last segment.
func relativePath(path, list string) string {
	if rest, ok := strings.CutPrefix(path, list+"."); ok {
		return rest
	}
	return path[strings.LastIndex(path, ".")+1:]
}

// isPathPrefix reports whether prefix is path or a dot-separated ancestor of it.
func isPathPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+".")
}

// hasPrefix reports whether prefix is the start of path.
func hasPrefix(path, prefix []string) bool {
	return len(prefix) <= len(path) && slices.Equal(path[:len(prefix)], prefix)
}

// astValue converts a coerced argument value to a GraphQL literal. typ, the
// consumer argument's type, decides between String and Enum literals and lets
// whole floats print as Int.
func astValue(value any, typ *ast.Type, schema *ast.Schema) (*ast.Value, error) {
	var named *ast.Definition
	if typ != nil {
		named = schema.Types[typ.Name()]
	}

	switch v := value.(type) {
	case nil:
		return &ast.Value{Kind: ast.NullValue, Raw: "null"}, nil
	case string:
		if named != nil && named.Kind == ast.Enum {
			return &ast.Value{Kind: ast.EnumValue, Raw: v}, nil
		}
		return &ast.Value{Kind: ast.StringValue, Raw: v}, nil
	case bool:
		return &ast.Value{Kind: ast.BooleanValue, Raw: strconv.FormatBool(v)}, nil
	case int:
		return &ast.Value{Kind: ast.IntValue, Raw: strconv.Itoa(v)}, nil
	case int64:
		return &ast.Value{Kind: ast.IntValue, Raw: strconv.FormatInt(v, 10)}, nil
	case float64:
		if typ != nil && typ.Elem == nil && typ.NamedType == "Int" && v == math.Trunc(v) {
			return &ast.Value{Kind: ast.IntValue, Raw: strconv.FormatFloat(v, 'f', -1, 64)}, nil
		}
		return &ast.Value{Kind: ast.FloatValue, Raw: strconv.FormatFloat(v, 'f', -1, 64)}, nil
	case json.Number:
		if _, err := v.Int64(); err == nil {
			return &ast.Value{Kind: ast.IntValue, Raw: v.String()}, nil
		}
		return &ast.Value{Kind: ast.FloatValue, Raw: v.String()}, nil
	case []any:
		var elem *ast.Type
		if typ != nil {
			elem = typ.Elem
		}
		list := &ast.Value{Kind: ast.ListValue}
		for _, item := range v {
			child, err := astValue(item, elem, schema)
			if err != nil {
				return nil, err
			}
			list.Children = append(list.Children, &ast.ChildValue{Value: child})
		}
		return list, nil
	case map[string]any:
		object := &ast.Value{Kind: ast.ObjectValue}
		for _, name := range slices.Sorted(maps.Keys(v)) {
			var fieldType *ast.Type
			if named != nil {
				if def := named.Fields.ForName(name); def != nil {
					fieldType = def.Type
				}
			}
			child, err := astValue(v[name], fieldType, schema)
			if err != nil {
				return nil, err
			}
			object.Children = append(object.Children, &ast.ChildValue{Name: name, Value: child})
		}
		return object, nil
	default:
		return nil, fmt.Errorf("unsupported value of type %T", value)
	}
}
