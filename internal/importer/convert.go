package importer

import (
	"regexp"
	"strings"

	"github.com/victor/mealie-importer/internal/mealie"
	"github.com/victor/mealie-importer/internal/mela"
)

var slugCleanup = regexp.MustCompile(`[^a-z0-9]+`)

func Convert(recipe mela.Recipe) mealie.Recipe {
	out := mealie.Recipe{
		Name:        strings.TrimSpace(recipe.Title),
		RecipeYield: strings.TrimSpace(recipe.Yield),
		TotalTime:   strings.TrimSpace(recipe.TotalTime),
		PrepTime:    strings.TrimSpace(recipe.PrepTime),
		CookTime:    strings.TrimSpace(recipe.CookTime),
		Description: strings.TrimSpace(recipe.Text),
		OrgURL:      strings.TrimSpace(recipe.Link),
		DateAdded:   recipe.DateAdded(),
		Extras: map[string]any{
			"mela_id":           strings.TrimSpace(recipe.ID),
			"mela_favorite":     recipe.Favorite,
			"mela_want_to_cook": recipe.WantToCook,
		},
	}

	for _, category := range recipe.Categories {
		category = strings.TrimSpace(category)
		if category == "" {
			continue
		}
		out.RecipeCategory = append(out.RecipeCategory, mealie.Organizer{
			Name: category,
			Slug: slugify(category),
		})
	}

	for _, line := range recipe.IngredientLines() {
		out.RecipeIngredient = append(out.RecipeIngredient, mealie.RecipeIngredient{
			Display:      line,
			OriginalText: line,
		})
	}

	for _, step := range recipe.InstructionSteps() {
		out.RecipeInstructions = append(out.RecipeInstructions, mealie.RecipeStep{
			Text: step,
		})
	}

	if strings.TrimSpace(recipe.Notes) != "" {
		out.Notes = append(out.Notes, mealie.RecipeNote{
			Title: "Mela Notes",
			Text:  strings.TrimSpace(recipe.Notes),
		})
	}
	if strings.TrimSpace(recipe.Nutrition) != "" {
		out.Notes = append(out.Notes, mealie.RecipeNote{
			Title: "Mela Nutrition",
			Text:  strings.TrimSpace(recipe.Nutrition),
		})
	}

	return out
}

func slugify(input string) string {
	slug := strings.ToLower(strings.TrimSpace(input))
	slug = slugCleanup.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}
