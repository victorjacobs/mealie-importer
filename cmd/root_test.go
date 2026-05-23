package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/victorjacobs/mealie-importer/internal/mealie"
	"github.com/victorjacobs/mealie-importer/internal/mela"
)

func TestPreviewImage(t *testing.T) {
	image := base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0x00})
	recipe := mela.Recipe{Images: []string{image, image}}

	preview, err := previewImage(context.Background(), recipe, true)
	require.NoError(t, err)
	assert.True(t, preview.Enabled)
	assert.True(t, preview.WillUpload)
	assert.Equal(t, 2, preview.ImageCount)
	assert.Equal(t, "image/jpeg", preview.MediaType)
	assert.Equal(t, "jpg", preview.Extension)
	assert.Equal(t, 4, preview.SizeBytes)
	assert.Empty(t, preview.SkippedReason)
}

func TestPreviewImageDisabled(t *testing.T) {
	preview, err := previewImage(context.Background(), mela.Recipe{Images: []string{"unused"}}, false)
	require.NoError(t, err)
	assert.False(t, preview.Enabled)
	assert.False(t, preview.WillUpload)
	assert.Equal(t, 1, preview.ImageCount)
	assert.Equal(t, "image upload disabled", preview.SkippedReason)
}

func TestPrintDryRunIncludesImageUpload(t *testing.T) {
	dir := t.TempDir()
	image := base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0x00})
	recipe := mela.Recipe{
		Path:   filepath.Join(dir, "recipe.melarecipe"),
		Title:  "Test Recipe",
		Images: []string{image},
	}

	oldStdout := os.Stdout
	read, write, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = write
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	require.NoError(t, printDryRun(context.Background(), []mela.Recipe{recipe}, true))
	require.NoError(t, write.Close())

	output, err := io.ReadAll(read)
	require.NoError(t, err)
	assert.Contains(t, string(output), `"imageUpload"`)
	assert.Contains(t, string(output), `"willUpload": true`)
	assert.Contains(t, string(output), `"recipe"`)
}

func TestUpsertRecipeUpdatesExistingRecipe(t *testing.T) {
	var created bool
	var updated mealie.Recipe
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/recipes":
			_, _ = w.Write([]byte(`{"items":[{"name":"Test Recipe","slug":"test-recipe"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/recipes":
			created = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`"new-recipe"`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/recipes/test-recipe":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&updated))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := mealie.NewClient(server.URL, "test-token")
	require.NoError(t, err)

	slug, wasCreated, err := upsertRecipe(context.Background(), client, mealie.Recipe{Name: "Test Recipe"})
	require.NoError(t, err)
	assert.False(t, wasCreated)
	assert.False(t, created)
	assert.Equal(t, "test-recipe", slug)
	assert.Equal(t, "Test Recipe", updated.Name)
	assert.Equal(t, "test-recipe", updated.Slug)
}

func TestUpsertRecipeCreatesMissingRecipe(t *testing.T) {
	var created mealie.CreateRecipe
	var updated mealie.Recipe
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/recipes":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/recipes":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&created))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`"new-recipe"`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/recipes/new-recipe":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&updated))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := mealie.NewClient(server.URL, "test-token")
	require.NoError(t, err)

	slug, wasCreated, err := upsertRecipe(context.Background(), client, mealie.Recipe{Name: "New Recipe"})
	require.NoError(t, err)
	assert.True(t, wasCreated)
	assert.Equal(t, "new-recipe", slug)
	assert.Equal(t, "New Recipe", created.Name)
	assert.Equal(t, "New Recipe", updated.Name)
	assert.Equal(t, "new-recipe", updated.Slug)
}

func TestUpsertRecipeUpdatesExistingStubBySlug(t *testing.T) {
	var updated mealie.Recipe
	var createAttempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/recipes":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/recipes":
			createAttempts++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`"new-recipe"`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/recipes/test-recipe":
			_, _ = w.Write([]byte(`{"name":"Test Recipe","slug":"test-recipe"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/recipes/test-recipe":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&updated))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := mealie.NewClient(server.URL, "test-token")
	require.NoError(t, err)

	slug, wasCreated, err := upsertRecipe(context.Background(), client, mealie.Recipe{Name: "Test Recipe"})
	require.NoError(t, err)
	assert.False(t, wasCreated)
	assert.Equal(t, 0, createAttempts)
	assert.Equal(t, "test-recipe", slug)
	assert.Equal(t, "Test Recipe", updated.Name)
	assert.Equal(t, "test-recipe", updated.Slug)
}

func TestUpsertRecipeRecoversExistingStubAfterCreateConflict(t *testing.T) {
	var updated mealie.Recipe
	var createAttempts int
	var slugLookups int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/recipes":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/recipes/test-recipe":
			slugLookups++
			if slugLookups == 1 {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"name":"Test Recipe","slug":"test-recipe"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/recipes":
			createAttempts++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":{"message":"Recipe already exists","error":true}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/recipes/test-recipe":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&updated))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := mealie.NewClient(server.URL, "test-token")
	require.NoError(t, err)

	slug, wasCreated, err := upsertRecipe(context.Background(), client, mealie.Recipe{Name: "Test Recipe"})
	require.NoError(t, err)
	assert.False(t, wasCreated)
	assert.Equal(t, 1, createAttempts)
	assert.Equal(t, 2, slugLookups)
	assert.Equal(t, "test-recipe", slug)
	assert.Equal(t, "Test Recipe", updated.Name)
	assert.Equal(t, "test-recipe", updated.Slug)
}

func TestPrepareImageConvertsHEICToJPEG(t *testing.T) {
	dir := t.TempDir()
	converter := filepath.Join(dir, "heif-dec")
	script := `#!/bin/sh
set -eu
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output)
      out="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf '\377\330\377converted' > "$out"
`
	require.NoError(t, os.WriteFile(converter, []byte(script), 0o700))
	t.Setenv("PATH", dir)

	heic := base64.StdEncoding.EncodeToString([]byte{
		0x00, 0x00, 0x00, 0x18,
		'f', 't', 'y', 'p',
		'h', 'e', 'i', 'c',
	})

	image, ok, err := prepareImage(context.Background(), mela.Recipe{Images: []string{heic}})
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "image/jpeg", image.MediaType)
	assert.Equal(t, "jpg", image.Extension)
	assert.Equal(t, "heic", image.ConvertedFrom)
	assert.Equal(t, []byte{0xff, 0xd8, 0xff, 'c', 'o', 'n', 'v', 'e', 'r', 't', 'e', 'd'}, image.Data)
}
