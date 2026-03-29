package swagger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolvedSpecFilesUsesExistingOutputPathAsBaseline(t *testing.T) {
	tempDir := newLocalTestDir(t)
	outputPath := filepath.Join(tempDir, "docs", "openapi.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(outputPath), 0755))
	require.NoError(t, os.WriteFile(outputPath, []byte("swagger: \"2.0\"\npaths: {}\n"), 0644))

	plugin := NewSwaggerPlugin()
	plugin.config = &SwaggerConfig{
		Enabled: true,
		Gen: GenConfig{
			Enabled:    true,
			OutputPath: outputPath,
		},
	}

	require.Equal(t, []string{outputPath}, plugin.resolvedSpecFiles())
}

func TestRebuildSwaggerDocsMergesExistingOutputFileAndAnnotations(t *testing.T) {
	tempDir := newLocalTestDir(t)
	docsDir := filepath.Join(tempDir, "docs")
	apiDir := filepath.Join(tempDir, "api")
	outputPath := filepath.Join(docsDir, "openapi.yaml")

	require.NoError(t, os.MkdirAll(docsDir, 0755))
	require.NoError(t, os.MkdirAll(apiDir, 0755))

	baseSpec := `swagger: "2.0"
info:
  title: Proto API
  version: "1.0.0"
paths:
  /proto/users:
    get:
      summary: Proto generated route
      responses:
        "200":
          description: ok
`
	require.NoError(t, os.WriteFile(outputPath, []byte(baseSpec), 0644))

	annotationFile := `package api

// CommentController exposes comment-generated routes.
type CommentController struct{}

// GetUser gets a user from annotation scan.
// @Summary Comment generated route
// @Description Added by annotation parser
// @Tags User
// @Produce json
// @Success 200 {object} map[string]string "ok"
// @Router /comment/users [get]
func (c *CommentController) GetUser() {}
`
	require.NoError(t, os.WriteFile(filepath.Join(apiDir, "handler.go"), []byte(annotationFile), 0644))

	plugin := NewSwaggerPlugin()
	plugin.config = &SwaggerConfig{
		Enabled: true,
		Info: InfoConfig{
			Title:   "Merged API",
			Version: "1.0.0",
		},
		UI: UIConfig{
			Enabled: false,
		},
		Gen: GenConfig{
			Enabled:    true,
			ScanDirs:   []string{apiDir},
			OutputPath: outputPath,
		},
	}
	plugin.swagger = plugin.newBaseSwaggerSpec()

	require.NoError(t, plugin.rebuildSwaggerDocs())
	require.Contains(t, plugin.swagger.Paths.Paths, "/proto/users")
	require.Contains(t, plugin.swagger.Paths.Paths, "/comment/users")

	require.NoError(t, plugin.saveSwaggerJSON())

	saved, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Contains(t, string(saved), "/proto/users")
	require.Contains(t, string(saved), "/comment/users")
}

func newLocalTestDir(t *testing.T) string {
	t.Helper()

	baseDir := filepath.Join(".", ".tmp-tests")
	require.NoError(t, os.MkdirAll(baseDir, 0755))

	tempDir, err := os.MkdirTemp(baseDir, "swagger-merge-*")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	return tempDir
}
