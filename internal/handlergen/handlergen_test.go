package handlergen

import (
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
