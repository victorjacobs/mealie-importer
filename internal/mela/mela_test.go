package mela

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadDirAndRecipeHelpers(t *testing.T) {
	dir := t.TempDir()
	image := base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0x00})
	data := `{
		"title": "Test Recipe",
		"date": 664544582.919013,
		"ingredients": "one\n\ntwo\n",
		"instructions": "step one\n\nstep two\nstep three",
		"images": ["` + image + `"]
	}`
	if err := os.WriteFile(filepath.Join(dir, "recipe.melarecipe"), []byte(data), 0o600); err != nil {
		require.NoError(t, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("{}"), 0o600); err != nil {
		require.NoError(t, err)
	}

	recipes, err := ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, recipes, 1)

	recipe := recipes[0]
	assert.Equal(t, "2022-01-22", recipe.DateAdded())
	assert.Equal(t, []string{"one", "two"}, recipe.IngredientLines())
	assert.Equal(t, []string{"step one", "step two", "step three"}, recipe.InstructionSteps())

	decoded, ok, err := recipe.PrimaryImage()
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "jpg", decoded.Extension)
	assert.Equal(t, "image/jpeg", decoded.MediaType)
}

func TestPrimaryImageDetectsHEIC(t *testing.T) {
	image := base64.StdEncoding.EncodeToString([]byte{
		0x00, 0x00, 0x00, 0x18,
		'f', 't', 'y', 'p',
		'h', 'e', 'i', 'c',
		0x00, 0x00, 0x00, 0x00,
	})

	decoded, ok, err := (Recipe{Images: []string{image}}).PrimaryImage()
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "heic", decoded.Extension)
	assert.Equal(t, "image/heic", decoded.MediaType)
}
