package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/victorjacobs/mealie-importer/internal/mela"
)

func TestConvert(t *testing.T) {
	got := Convert(mela.Recipe{
		Title:        "Chilli Con Carne",
		Yield:        "5 - 6 people",
		TotalTime:    "40min",
		PrepTime:     "10min",
		CookTime:     "30min",
		Text:         "A saucy chili.",
		Link:         "https://example.com/recipe",
		Date:         664544582.919013,
		Categories:   []string{"Dinner Ideas"},
		Ingredients:  "1 tbsp olive oil\n2 onions",
		Instructions: "Cook onions.\n\nSimmer.\nServe.",
		Notes:        "Use good beans.",
		Nutrition:    "**Calories** 367 kcal",
		ID:           "example",
		Favorite:     true,
	})

	assert.Equal(t, "Chilli Con Carne", got.Name)
	assert.Equal(t, "2022-01-22", got.DateAdded)
	require.Len(t, got.RecipeCategory, 1)
	assert.Equal(t, "dinner-ideas", got.RecipeCategory[0].Slug)
	require.Len(t, got.RecipeIngredient, 2)
	assert.Equal(t, "1 tbsp olive oil", got.RecipeIngredient[0].OriginalText)
	assert.Equal(t, "1 tbsp olive oil", got.RecipeIngredient[0].Note)
	require.Len(t, got.RecipeInstructions, 3)
	assert.Equal(t, "Simmer.", got.RecipeInstructions[1].Text)
	require.Len(t, got.Notes, 2)
	assert.Equal(t, "example", got.Extras["mela_id"])
	assert.Equal(t, "true", got.Extras["mela_favorite"])
	assert.Equal(t, "false", got.Extras["mela_want_to_cook"])
}
