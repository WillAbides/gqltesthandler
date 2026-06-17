package handlergen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name        string
		testdataDir string
	}{
		{
			name:        "simple_query",
			testdataDir: "testdata/simple_query",
		},
		{
			name:        "with_mutations",
			testdataDir: "testdata/with_mutations",
		},
		{
			name:        "with_fragments",
			testdataDir: "testdata/with_fragments",
		},
		{
			name:        "with_typename",
			testdataDir: "testdata/with_typename",
		},
		{
			name:        "with_aliases",
			testdataDir: "testdata/with_aliases",
		},
		{
			name:        "with_directives",
			testdataDir: "testdata/with_directives",
		},
		{
			name:        "abstract_nullability",
			testdataDir: "testdata/abstract_nullability",
		},
		{
			name:        "overlapping_fragments",
			testdataDir: "testdata/overlapping_fragments",
		},
		{
			name:        "interface_in_union",
			testdataDir: "testdata/interface_in_union",
		},
		{
			name:        "real_typename_field",
			testdataDir: "testdata/real_typename_field",
		},
		{
			name:        "with_interfaces",
			testdataDir: "testdata/with_interfaces",
		},
		{
			name:        "with_abstract_edges",
			testdataDir: "testdata/with_abstract_edges",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(test.testdataDir)
			outputDir := filepath.Join(t.TempDir(), "generated")
			require.NoError(t, os.MkdirAll(outputDir, 0o755))

			err := Run("schema.graphqls", "operations.graphql", outputDir)
			require.NoError(t, err)

			if os.Getenv("UPDATE_SNAPS") != "" {
				require.NoError(t, os.RemoveAll("generated"))
				require.NoError(t, os.MkdirAll("generated", 0o755))
				require.NoError(t, copyDir(outputDir, "generated"))
			}

			assertEqualDir(t, "generated", outputDir)
		})
	}
}

func getDirFiles(t require.TestingT, dir string) []string {
	var files []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, relPath)
		return nil
	}))
	return files
}

func assertEqualDir(t *testing.T, expectedDir, actualDir string) {
	t.Helper()

	expectedFiles := getDirFiles(t, expectedDir)

	assert.Equal(t, expectedFiles, getDirFiles(t, actualDir), "file lists do not match")

	for _, filename := range expectedFiles {
		expectedContent, err := os.ReadFile(filepath.Join(expectedDir, filename))
		require.NoError(t, err)

		actualContent, err := os.ReadFile(filepath.Join(actualDir, filename))
		if assert.NoError(t, err) {
			assert.Equal(t, string(expectedContent), string(actualContent), "file contents do not match for %s", filename)
		}
	}
}

func TestRunAliasConflict(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.graphqls")
	operationsPath := filepath.Join(dir, "operations.graphql")

	schema := `type Query {
  user(id: ID!): User
}

type User {
  id: ID!
  name: String!
}
`
	// "name" and the alias "Name" are distinct GraphQL response keys, but both
	// map to the Go field name "Name", so generation must fail loudly rather
	// than emit a struct with duplicate fields.
	operations := `query Collide($id: ID!) {
  user(id: $id) {
    name
    Name: id
  }
}
`
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o600))
	require.NoError(t, os.WriteFile(operationsPath, []byte(operations), 0o600))

	err := Run(schemaPath, operationsPath, filepath.Join(dir, "generated"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `Go field name "Name"`)
	assert.Contains(t, err.Error(), `"name"`)
	assert.Contains(t, err.Error(), `"Name"`)
}

func TestTypenameCollision(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.graphqls")
	operationsPath := filepath.Join(dir, "operations.graphql")

	schema := `type Query {
  thing: Thing
}

type Thing {
  id: ID!
  typename: String!
}
`
	operations := `query TypenameCollide {
  thing {
    __typename
    typename
  }
}
`
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o600))
	require.NoError(t, os.WriteFile(operationsPath, []byte(operations), 0o600))

	err := Run(schemaPath, operationsPath, filepath.Join(dir, "generated"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `Go field name "Typename"`)
	assert.Contains(t, err.Error(), `"__typename"`)
	assert.Contains(t, err.Error(), `"typename"`)
}

func TestExportedName(t *testing.T) {
	t.Run("__typename becomes Typename", func(t *testing.T) {
		assert.Equal(t, "Typename", exportedName("__typename"))
	})

	t.Run("id becomes ID", func(t *testing.T) {
		assert.Equal(t, "ID", exportedName("id"))
	})

	t.Run("regular field uppercases first rune", func(t *testing.T) {
		assert.Equal(t, "Name", exportedName("name"))
	})

	t.Run("empty string unchanged", func(t *testing.T) {
		assert.Equal(t, "", exportedName(""))
	})
}

// TestRun_SynthesizedTypenameConflict pins that a flat (shared-only) abstract
// selection whose synthesized __typename discriminator collides with another
// response key mapping to Go field `Typename` fails generation, naming the
// colliding response keys. The discriminator is added after extractSelectionFields
// runs, so its response-key collision check cannot catch it; withFlatTypename
// reports it instead. The explicit-__typename and plain alias collisions are
// already covered by TestTypenameCollision and TestRunAliasConflict.
func TestRun_SynthesizedTypenameConflict(t *testing.T) {
	schema := `type Query {
  node(id: ID!): Node
}
interface Node {
  id: ID!
  typename: String!
}
type User implements Node {
  id: ID!
  typename: String!
  login: String!
}
`
	tests := []struct {
		name       string
		operations string
		// wantKey is the response key the error must report for the existing
		// field. For an alias it is the alias, not the underlying field name.
		wantKey string
	}{
		{
			// A real `typename` field: its response key equals its field name, so
			// the synthesized discriminator collides on response key "typename".
			name: "real typename field",
			operations: `query Q($id: ID!) {
  node(id: $id) {
    typename
    id
  }
}
`,
			wantKey: "typename",
		},
		{
			// `Typename: id` aliases id to response key "Typename", which collides
			// with the synthesized discriminator by response key. The error must
			// name the alias "Typename", not the underlying field "id".
			name: "aliased response key",
			operations: `query Q($id: ID!) {
  node(id: $id) {
    Typename: id
  }
}
`,
			wantKey: "Typename",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			schemaPath := filepath.Join(dir, "schema.graphqls")
			opsPath := filepath.Join(dir, "operations.graphql")
			require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o600))
			require.NoError(t, os.WriteFile(opsPath, []byte(test.operations), 0o600))

			err := Run(schemaPath, opsPath, filepath.Join(dir, "generated"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), `Go field name "Typename"`)
			assert.Contains(t, err.Error(), fmt.Sprintf(`response keys %q and "__typename"`, test.wantKey))
		})
	}
}

// copyDir copies all files from srcDir to dstDir
func copyDir(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, 0o644)
	})
}
