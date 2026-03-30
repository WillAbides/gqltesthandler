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
		responseFields := extractSelectionFields(schema, op.SelectionSet, op.Name+"Response")

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
	NestedFields []selectionField
	TypeName     string // for nested object types, the generated struct name
}

func extractSelectionFields(schema *ast.Schema, selSet ast.SelectionSet, prefix string) []selectionField {
	var fields []selectionField
	seen := map[string]bool{}
	extractSelectionFieldsInto(schema, selSet, prefix, &fields, seen)
	return fields
}

func extractSelectionFieldsInto(schema *ast.Schema, selSet ast.SelectionSet, prefix string, fields *[]selectionField, seen map[string]bool) {
	for _, sel := range selSet {
		switch sel := sel.(type) {
		case *ast.Field:
			if seen[sel.Name] {
				continue
			}
			seen[sel.Name] = true

			typeDef := schema.Types[sel.Definition.Type.Name()]
			isObject := typeDef != nil && (typeDef.Kind == ast.Object || typeDef.Kind == ast.Interface || typeDef.Kind == ast.Union)

			sf := selectionField{
				GraphQLName: sel.Name,
				GoName:      exportedName(sel.Name),
				JSONName:    sel.Name,
			}

			if isObject && len(sel.SelectionSet) > 0 {
				nestedTypeName := prefix + exportedName(sel.Name)
				sf.TypeName = nestedTypeName
				sf.NestedFields = extractSelectionFields(schema, sel.SelectionSet, nestedTypeName)
				sf.GoType = wrapGoType(sel.Definition.Type, nestedTypeName)
			} else {
				sf.GoType = goTypeForGraphQL(schema, sel.Definition.Type)
			}

			*fields = append(*fields, sf)
		case *ast.FragmentSpread:
			if sel.Definition != nil {
				extractSelectionFieldsInto(schema, sel.Definition.SelectionSet, prefix, fields, seen)
			}
		case *ast.InlineFragment:
			extractSelectionFieldsInto(schema, sel.SelectionSet, prefix, fields, seen)
		}
	}
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

	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "types.tmpl", map[string]any{
		"PackageName": packageName,
		"Operations":  ops,
		"InputTypes":  inputTypes,
		"EnumTypes":   enumTypes,
	})
	if err != nil {
		return "", fmt.Errorf("executing types template: %w", err)
	}
	return buf.String(), nil
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
