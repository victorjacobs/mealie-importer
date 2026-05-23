package main

import (
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/victor/mealie-importer/internal/mela"
)

func TestPreviewImage(t *testing.T) {
	image := base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0x00})
	recipe := mela.Recipe{Images: []string{image, image}}

	preview, err := previewImage(recipe, true)
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
	preview, err := previewImage(mela.Recipe{Images: []string{"unused"}}, false)
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

	require.NoError(t, printDryRun([]mela.Recipe{recipe}, true))
	require.NoError(t, write.Close())

	output, err := io.ReadAll(read)
	require.NoError(t, err)
	assert.Contains(t, string(output), `"imageUpload"`)
	assert.Contains(t, string(output), `"willUpload": true`)
	assert.Contains(t, string(output), `"recipe"`)
}
