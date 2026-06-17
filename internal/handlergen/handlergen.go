package handlergen

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"golang.org/x/tools/imports"
	"mvdan.cc/gofumpt/format"
)

//go:embed helpers/helpers.go
var helpersSource []byte

//go:embed templates
var templatesFS embed.FS

var templates = template.Must(
	template.New("").
		Funcs(template.FuncMap{
			"lcFirst": lcFirst,
		}).
		ParseFS(templatesFS, "templates/*.tmpl"),
)

func lcFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func Run(schemaPath, operationsPath, outputPath string) error {
	schema, err := loadSchema(schemaPath)
	if err != nil {
		return err
	}

	ops, err := loadOperations(schema, operationsPath)
	if err != nil {
		return err
	}

	packageName := detectPackageName(outputPath)

	err = os.MkdirAll(outputPath, 0o750)
	if err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	err = writeHelpers(packageName, outputPath)
	if err != nil {
		return fmt.Errorf("writing helpers.go: %w", err)
	}

	typesCode, err := buildTypes(ops, schema, packageName)
	if err != nil {
		return fmt.Errorf("building types: %w", err)
	}
	err = writeFile(filepath.Join(outputPath, "types_gen.go"), []byte(typesCode))
	if err != nil {
		return fmt.Errorf("writing types_gen.go: %w", err)
	}

	handlerCode, err := buildHandler(ops, packageName)
	if err != nil {
		return fmt.Errorf("building handler: %w", err)
	}
	err = writeFile(filepath.Join(outputPath, "handler.go"), []byte(handlerCode))
	if err != nil {
		return fmt.Errorf("writing handler.go: %w", err)
	}

	serverCode, err := buildServer(ops, packageName)
	if err != nil {
		return fmt.Errorf("building server: %w", err)
	}
	err = writeFile(filepath.Join(outputPath, "server.go"), []byte(serverCode))
	if err != nil {
		return fmt.Errorf("writing server.go: %w", err)
	}

	return nil
}

func loadSchema(schemaPath string) (*ast.Schema, error) {
	src, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("reading schema file %q: %w", schemaPath, err)
	}
	schema, gqlErr := gqlparser.LoadSchema(&ast.Source{
		Name:  schemaPath,
		Input: string(src),
	})
	if gqlErr != nil {
		return nil, fmt.Errorf("parsing schema: %s", gqlErr.Error())
	}
	return schema, nil
}

func loadOperations(schema *ast.Schema, operationsPath string) ([]operationData, error) {
	src, err := os.ReadFile(operationsPath)
	if err != nil {
		return nil, fmt.Errorf("reading operations file %q: %w", operationsPath, err)
	}
	doc, gqlErr := gqlparser.LoadQuery(schema, string(src))
	if gqlErr != nil {
		return nil, fmt.Errorf("parsing operations: %s", gqlErr.Error())
	}

	var ops []operationData
	for _, op := range doc.Operations {
		if op.Name == "" {
			return nil, fmt.Errorf("anonymous operations are not supported; all operations must be named")
		}

		vars := extractVariables(schema, op)
		responseFields, fieldsErr := extractSelectionFields(schema, op.SelectionSet, op.Name+"Response")
		if fieldsErr != nil {
			return nil, fmt.Errorf("operation %q: %w", op.Name, fieldsErr)
		}

		ops = append(ops, operationData{
			Name:             op.Name,
			LowercaseName:    lcFirst(op.Name),
			OperationType:    string(op.Operation),
			Variables:        vars,
			ResponseFields:   responseFields,
			VariablesType:    op.Name + "Variables",
			ResponseType:     op.Name + "Response",
			ResultInterface:  lcFirst(op.Name) + "Result",
			ExpectationField: lcFirst(op.Name) + "ExpectResponses",
			BuilderTypeName:  op.Name + "Expectation",
		})
	}

	return ops, nil
}

func extractVariables(schema *ast.Schema, op *ast.OperationDefinition) []variableData {
	var vars []variableData
	for _, v := range op.VariableDefinitions {
		vars = append(vars, variableData{
			GraphQLName: v.Variable,
			GoName:      exportedName(v.Variable),
			GoType:      goTypeForGraphQL(schema, v.Type),
			JSONName:    v.Variable,
		})
	}
	return vars
}

type selectionField struct {
	GraphQLName  string
	GoName       string
	GoType       string
	JSONName     string
	OmitEmpty    bool
	NestedFields []selectionField
	TypeName     string // for nested object types, the generated struct name

	// TypenameValues, when set, marks this field as the synthesized __typename
	// discriminator of a flat abstract struct and lists its constant values.
	TypenameValues []string

	// IsAbstract marks an interface/union selection modeled as a discriminator
	// interface plus one variant struct per narrowing concrete type.
	IsAbstract     bool
	InterfaceName  string
	SentinelMethod string
	Variants       []abstractVariant
}

// abstractVariant is one concrete type condition of an abstract selection.
type abstractVariant struct {
	GraphQLType    string // concrete GraphQL type name, also the __typename value
	StructName     string // generated struct name
	InterfaceName  string // interface this variant implements
	SentinelMethod string // marker method to implement
	Fields         []selectionField
}

// responseKey is the alias when present, else the field name. GraphQL keys
// response objects by it, while schema/type lookup still uses sel.Name.
func responseKey(sel *ast.Field) string {
	if sel.Alias != "" {
		return sel.Alias
	}
	return sel.Name
}

func extractSelectionFields(schema *ast.Schema, selSet ast.SelectionSet, prefix string) ([]selectionField, error) {
	var fields []selectionField
	seen := map[string]bool{}
	goNames := map[string]string{}
	err := extractSelectionFieldsInto(schema, selSet, prefix, &fields, seen, goNames)
	if err != nil {
		return nil, err
	}
	return fields, nil
}

// extractSelectionFieldsInto fails when two response keys map to the same Go
// field name, which would produce duplicate struct fields.
func extractSelectionFieldsInto(schema *ast.Schema, selSet ast.SelectionSet, prefix string, fields *[]selectionField, seen map[string]bool, goNames map[string]string) error {
	for _, sel := range selSet {
		switch sel := sel.(type) {
		case *ast.Field:
			key := responseKey(sel)
			if seen[key] {
				continue
			}
			goName := exportedName(key)
			prevKey, collides := goNames[goName]
			if collides {
				return fmt.Errorf("response keys %q and %q both map to Go field name %q in %s", prevKey, key, goName, prefix)
			}
			seen[key] = true
			goNames[goName] = key

			typeDef := schema.Types[sel.Definition.Type.Name()]
			isComposite := typeDef != nil && (typeDef.Kind == ast.Object || typeDef.Kind == ast.Interface || typeDef.Kind == ast.Union)
			isAbstract := typeDef != nil && (typeDef.Kind == ast.Interface || typeDef.Kind == ast.Union)

			if isAbstract && len(sel.SelectionSet) > 0 {
				abstractField, ok, abErr := buildAbstractField(schema, sel, prefix)
				if abErr != nil {
					return abErr
				}
				if ok {
					*fields = append(*fields, abstractField)
					continue
				}
			}

			sf := selectionField{
				GraphQLName: sel.Name,
				GoName:      goName,
				JSONName:    key,
			}

			switch {
			case isComposite && len(sel.SelectionSet) > 0:
				nestedTypeName := prefix + goName
				sf.TypeName = nestedTypeName
				nested, err := extractSelectionFields(schema, sel.SelectionSet, nestedTypeName)
				if err != nil {
					return err
				}
				sf.NestedFields = nested
				sf.GoType = wrapGoType(sel.Definition.Type, nestedTypeName)
				if isAbstract {
					sf.NestedFields, err = withFlatTypename(schema, sel.Definition.Type.Name(), nestedTypeName, sf.NestedFields)
					if err != nil {
						return err
					}
				}
			case sel.Name == "__typename":
				// The `__typename` meta-field is `String!` (non-null) per the
				// GraphQL spec, so emit a plain string rather than *string.
				sf.GoType = "string"
			default:
				sf.GoType = goTypeForGraphQL(schema, sel.Definition.Type)
			}

			*fields = append(*fields, sf)
		case *ast.FragmentSpread:
			if sel.Definition != nil {
				err := extractSelectionFieldsInto(schema, sel.Definition.SelectionSet, prefix, fields, seen, goNames)
				if err != nil {
					return err
				}
			}
		case *ast.InlineFragment:
			err := extractSelectionFieldsInto(schema, sel.SelectionSet, prefix, fields, seen, goNames)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// withFlatTypename adds a settable `<Struct>Typename` discriminator to a flat
// abstract struct, since genqlient injects and requires `__typename` for every
// interface/union field. An explicit `__typename` is replaced in place; a
// synthesized one uses `omitempty` so fixtures can stay silent. A synthesized
// discriminator that collides with a real `typename` field is rejected here.
func withFlatTypename(schema *ast.Schema, abstractType, structName string, fields []selectionField) ([]selectionField, error) {
	typenameField := selectionField{
		GraphQLName:    "__typename",
		GoName:         "Typename",
		GoType:         structName + "Typename",
		JSONName:       "__typename",
		TypenameValues: possibleConcreteTypes(schema, abstractType),
	}
	for i, f := range fields {
		if f.JSONName == "__typename" {
			fields[i] = typenameField
			return fields, nil
		}
	}
	for _, f := range fields {
		if f.GoName == typenameField.GoName {
			return nil, fmt.Errorf("response keys %q and %q both map to Go field name %q in %s", f.JSONName, typenameField.JSONName, typenameField.GoName, structName)
		}
	}
	typenameField.OmitEmpty = true
	return append([]selectionField{typenameField}, fields...), nil
}

// buildAbstractField models an interface/union selection as a discriminator
// interface plus one variant struct per narrowing concrete type. A fragment's
// fields attach only to its possible types (the intersection with the parent's),
// so they never leak onto sibling variants; fields selected directly on the
// abstract field are shared across every variant. A selection with no narrowing
// fragment returns false, and the caller emits a single flat struct instead.
func buildAbstractField(schema *ast.Schema, sel *ast.Field, prefix string) (selectionField, bool, error) {
	parentOrdered := possibleConcreteTypes(schema, sel.Definition.Type.Name())
	parentSet := typeSet(parentOrdered)

	var shared ast.SelectionSet
	var order []string
	seen := map[string]bool{}
	variantSels := map[string]ast.SelectionSet{}

	recordOrder := func(applicable map[string]bool) {
		for _, t := range parentOrdered {
			if applicable[t] && !seen[t] {
				seen[t] = true
				order = append(order, t)
			}
		}
	}

	var walk func(selSet ast.SelectionSet, applicable map[string]bool, parentScope bool)
	walk = func(selSet ast.SelectionSet, applicable map[string]bool, parentScope bool) {
		for _, s := range selSet {
			switch s := s.(type) {
			case *ast.Field:
				// Unaliased __typename is injected by each variant's MarshalJSON;
				// an aliased one (`tag: __typename`) stays a normal response key.
				if s.Name == "__typename" && responseKey(s) == "__typename" {
					continue
				}
				if parentScope {
					shared = append(shared, s)
					continue
				}
				for _, t := range parentOrdered {
					if applicable[t] {
						variantSels[t] = append(variantSels[t], s)
					}
				}
			case *ast.InlineFragment:
				cond := intersectTypes(applicable, possibleConcreteTypes(schema, s.TypeCondition))
				if sameTypeSet(cond, applicable) {
					walk(s.SelectionSet, applicable, parentScope)
					continue
				}
				recordOrder(cond)
				walk(s.SelectionSet, cond, false)
			case *ast.FragmentSpread:
				if s.Definition == nil {
					continue
				}
				cond := intersectTypes(applicable, possibleConcreteTypes(schema, s.Definition.TypeCondition))
				if sameTypeSet(cond, applicable) {
					walk(s.Definition.SelectionSet, applicable, parentScope)
					continue
				}
				recordOrder(cond)
				walk(s.Definition.SelectionSet, cond, false)
			}
		}
	}
	walk(sel.SelectionSet, parentSet, true)

	if len(order) == 0 {
		return selectionField{}, false, nil
	}

	key := responseKey(sel)
	interfaceName := prefix + exportedName(key)
	sentinel := "is" + interfaceName

	sf := selectionField{
		GraphQLName:    sel.Name,
		GoName:         exportedName(key),
		JSONName:       key,
		GoType:         wrapInterfaceGoType(sel.Definition.Type, interfaceName),
		IsAbstract:     true,
		InterfaceName:  interfaceName,
		SentinelMethod: sentinel,
	}

	for _, typeCond := range order {
		structName := interfaceName + exportedName(typeCond)

		combined := make(ast.SelectionSet, 0, len(shared)+len(variantSels[typeCond]))
		combined = append(combined, shared...)
		combined = append(combined, variantSels[typeCond]...)

		variantFields, err := extractSelectionFields(schema, combined, structName)
		if err != nil {
			return selectionField{}, false, err
		}
		sf.Variants = append(sf.Variants, abstractVariant{
			GraphQLType:    typeCond,
			StructName:     structName,
			InterfaceName:  interfaceName,
			SentinelMethod: sentinel,
			Fields:         variantFields,
		})
	}

	return sf, true, nil
}

// possibleConcreteTypes returns, in schema declaration order, the names of the
// concrete object types a type condition can resolve to: the object itself for
// an object type, the implementors for an interface, and the members for a
// union.
func possibleConcreteTypes(schema *ast.Schema, name string) []string {
	def := schema.Types[name]
	if def == nil {
		return nil
	}
	var out []string
	for _, pt := range schema.GetPossibleTypes(def) {
		if pt.Kind == ast.Object {
			out = append(out, pt.Name)
		}
	}
	return out
}

func typeSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// intersectTypes returns the subset of condTypes that are also in applicable.
func intersectTypes(applicable map[string]bool, condTypes []string) map[string]bool {
	out := map[string]bool{}
	for _, t := range condTypes {
		if applicable[t] {
			out[t] = true
		}
	}
	return out
}

// sameTypeSet reports whether two concrete-type sets have identical membership.
func sameTypeSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for t := range a {
		if !b[t] {
			return false
		}
	}
	return true
}

// wrapInterfaceGoType wraps an interface-typed selection. Interfaces are nilable
// in Go, so a nil value already represents a null response and no pointer is
// added; list nesting is preserved.
func wrapInterfaceGoType(gqlType *ast.Type, goTypeName string) string {
	if gqlType.Elem != nil {
		return "[]" + wrapInterfaceGoType(gqlType.Elem, goTypeName)
	}
	return goTypeName
}

// wrapGoType wraps a named type with pointer/slice based on the GraphQL type's nullability and list nesting.
func wrapGoType(gqlType *ast.Type, goTypeName string) string {
	if gqlType.Elem != nil {
		inner := wrapGoType(gqlType.Elem, goTypeName)
		if gqlType.NonNull {
			return "[]" + inner
		}
		return "[]" + inner
	}
	if gqlType.NonNull {
		return goTypeName
	}
	return "*" + goTypeName
}

func goTypeForGraphQL(schema *ast.Schema, gqlType *ast.Type) string {
	if gqlType.Elem != nil {
		inner := goTypeForGraphQL(schema, gqlType.Elem)
		return "[]" + inner
	}

	baseType := scalarGoType(schema, gqlType.NamedType)

	if gqlType.NonNull {
		return baseType
	}
	// Nullable scalars become pointers
	return "*" + baseType
}

func scalarGoType(schema *ast.Schema, name string) string {
	switch name {
	case "String", "ID":
		return "string"
	case "Int":
		return "int"
	case "Float":
		return "float64"
	case "Boolean":
		return "bool"
	default:
		// Check if it's an enum
		typeDef := schema.Types[name]
		if typeDef != nil && typeDef.Kind == ast.Enum {
			return name
		}
		// Check if it's an input object
		if typeDef != nil && typeDef.Kind == ast.InputObject {
			return name
		}
		// Custom scalar or unknown → any
		return "any"
	}
}

func exportedName(s string) string {
	if s == "" {
		return s
	}
	// GraphQL's `__typename` meta-field is unexported under the default
	// rune-uppercase rule, which trips go vet's "structtag" check when
	// emitted alongside a json tag. Mirror genqlient's mapping
	// (Typename + json:"__typename") so unions/interfaces work without
	// downstream patching.
	if s == "__typename" {
		return "Typename"
	}
	// Handle common GraphQL conventions like camelCase
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	// Handle ID specially
	if s == "id" {
		return "ID"
	}
	return string(runes)
}

type operationData struct {
	Name             string
	LowercaseName    string
	OperationType    string // "query" or "mutation"
	Variables        []variableData
	ResponseFields   []selectionField
	VariablesType    string
	ResponseType     string
	ResultInterface  string
	ExpectationField string
	BuilderTypeName  string
}

type variableData struct {
	GraphQLName string
	GoName      string
	GoType      string
	JSONName    string
}

func buildTypes(ops []operationData, schema *ast.Schema, packageName string) (string, error) {
	// Collect all input types and enums referenced by operations
	inputTypes := collectInputTypes(ops, schema)
	enumTypes := collectEnumTypes(ops, schema)
	typenameTypes := collectTypenameTypes(ops)

	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "types.tmpl", map[string]any{
		"PackageName":   packageName,
		"Operations":    ops,
		"InputTypes":    inputTypes,
		"EnumTypes":     enumTypes,
		"TypenameTypes": typenameTypes,
	})
	if err != nil {
		return "", fmt.Errorf("executing types template: %w", err)
	}
	return buf.String(), nil
}

// collectTypenameTypes gathers the synthesized `<Struct>Typename` discriminator
// types of flat abstract structs (see withFlatTypename) so the template can emit
// each as a defined string plus one constant per possible concrete type. The
// shape matches enums, so enumTypeData is reused.
func collectTypenameTypes(ops []operationData) []enumTypeData {
	seen := map[string]bool{}
	var result []enumTypeData
	for _, op := range ops {
		collectTypenameFromFields(op.ResponseFields, seen, &result)
	}
	return result
}

func collectTypenameFromFields(fields []selectionField, seen map[string]bool, result *[]enumTypeData) {
	for _, f := range fields {
		if len(f.TypenameValues) > 0 && !seen[f.GoType] {
			seen[f.GoType] = true
			var values []enumValueData
			for _, v := range f.TypenameValues {
				values = append(values, enumValueData{
					GoName: f.GoType + exportedName(v),
					Value:  v,
				})
			}
			*result = append(*result, enumTypeData{Name: f.GoType, Values: values})
		}
		if len(f.NestedFields) > 0 {
			collectTypenameFromFields(f.NestedFields, seen, result)
		}
		for _, v := range f.Variants {
			collectTypenameFromFields(v.Fields, seen, result)
		}
	}
}

type inputTypeData struct {
	Name   string
	Fields []inputFieldData
}

type inputFieldData struct {
	GoName    string
	GoType    string
	JSONName  string
	OmitEmpty bool
}

type enumTypeData struct {
	Name   string
	Values []enumValueData
}

type enumValueData struct {
	GoName string
	Value  string
}

func collectInputTypes(ops []operationData, schema *ast.Schema) []inputTypeData {
	seen := map[string]bool{}
	var result []inputTypeData
	for _, op := range ops {
		for _, v := range op.Variables {
			collectInputTypesRecursive(schema, v.GoType, seen, &result)
		}
	}
	return result
}

func collectInputTypesRecursive(schema *ast.Schema, goType string, seen map[string]bool, result *[]inputTypeData) {
	// Strip pointer and slice prefixes to get the base type name
	name := strings.TrimPrefix(goType, "*")
	name = strings.TrimPrefix(name, "[]")
	name = strings.TrimPrefix(name, "*")

	if seen[name] {
		return
	}
	typeDef := schema.Types[name]
	if typeDef == nil || typeDef.Kind != ast.InputObject {
		return
	}
	seen[name] = true

	var fields []inputFieldData
	for _, f := range typeDef.Fields {
		ft := goTypeForGraphQL(schema, f.Type)
		fields = append(fields, inputFieldData{
			GoName:    exportedName(f.Name),
			GoType:    ft,
			JSONName:  f.Name,
			OmitEmpty: !f.Type.NonNull,
		})
		// Recurse into nested input types
		collectInputTypesRecursive(schema, ft, seen, result)
	}
	*result = append(*result, inputTypeData{
		Name:   name,
		Fields: fields,
	})
}

func collectEnumTypes(ops []operationData, schema *ast.Schema) []enumTypeData {
	seen := map[string]bool{}
	var result []enumTypeData

	// Check all variable types and response fields for enum references
	for _, op := range ops {
		for _, v := range op.Variables {
			collectEnumFromType(schema, v.GoType, seen, &result)
		}
		collectEnumFromFields(schema, op.ResponseFields, seen, &result)
	}
	return result
}

func collectEnumFromType(schema *ast.Schema, goType string, seen map[string]bool, result *[]enumTypeData) {
	name := strings.TrimPrefix(goType, "*")
	name = strings.TrimPrefix(name, "[]")
	name = strings.TrimPrefix(name, "*")

	if seen[name] {
		return
	}
	typeDef := schema.Types[name]
	if typeDef == nil || typeDef.Kind != ast.Enum {
		return
	}
	// Skip built-in GraphQL enums
	if typeDef.BuiltIn {
		return
	}
	seen[name] = true

	var values []enumValueData
	for _, v := range typeDef.EnumValues {
		values = append(values, enumValueData{
			GoName: name + exportedName(strings.ToLower(v.Name)),
			Value:  v.Name,
		})
	}
	*result = append(*result, enumTypeData{
		Name:   name,
		Values: values,
	})
}

func collectEnumFromFields(schema *ast.Schema, fields []selectionField, seen map[string]bool, result *[]enumTypeData) {
	for _, f := range fields {
		collectEnumFromType(schema, f.GoType, seen, result)
		if len(f.NestedFields) > 0 {
			collectEnumFromFields(schema, f.NestedFields, seen, result)
		}
		for _, v := range f.Variants {
			collectEnumFromFields(schema, v.Fields, seen, result)
		}
	}
}

func buildHandler(ops []operationData, packageName string) (string, error) {
	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "handler.tmpl", map[string]any{
		"PackageName": packageName,
		"Operations":  ops,
	})
	if err != nil {
		return "", fmt.Errorf("executing handler template: %w", err)
	}
	return buf.String(), nil
}

func buildServer(ops []operationData, packageName string) (string, error) {
	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "server.tmpl", map[string]any{
		"PackageName": packageName,
		"Operations":  ops,
	})
	if err != nil {
		return "", fmt.Errorf("executing server template: %w", err)
	}
	return buf.String(), nil
}

func detectPackageName(outDir string) string {
	if !filepath.IsAbs(outDir) && !strings.HasPrefix(outDir, ".") {
		return sanitizePackageName(filepath.Base(outDir))
	}
	abs, err := filepath.Abs(filepath.FromSlash(outDir))
	if err != nil {
		return "testhandler"
	}
	return sanitizePackageName(filepath.Base(abs))
}

func sanitizePackageName(name string) string {
	var result []rune
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			result = append(result, unicode.ToLower(r))
		}
	}
	if len(result) == 0 || unicode.IsDigit(result[0]) {
		return "testhandler"
	}
	return string(result)
}

func writeHelpers(packageName, outDir string) error {
	source := bytes.Replace(helpersSource, []byte("package helpers"), []byte("package "+packageName), 1)
	filename := filepath.Join(outDir, "helpers.go")
	return writeFile(filename, source)
}

func writeFile(filename string, content []byte) (errOut error) {
	const header = "// Code generated by github.com/willabides/gqltesthandler/cmd/gqltesthandler. DO NOT EDIT.\n\n"

	source, err := imports.Process(filename, append([]byte(header), content...), nil)
	if err != nil {
		return fmt.Errorf("running goimports: %w", err)
	}
	source, err = format.Source(source, format.Options{ExtraRules: true})
	if err != nil {
		return fmt.Errorf("running gofumpt: %w", err)
	}

	out, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}

	defer func() { errOut = errors.Join(errOut, out.Close()) }()

	_, err = out.Write(source)
	if err != nil {
		return fmt.Errorf("writing to output file: %w", err)
	}

	return nil
}
