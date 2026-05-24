package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/victorjacobs/mealie-importer/internal/imageconv"
	"github.com/victorjacobs/mealie-importer/internal/importer"
	"github.com/victorjacobs/mealie-importer/internal/mealie"
	"github.com/victorjacobs/mealie-importer/internal/mela"
	"go.uber.org/zap"
)

var slugCleanup = regexp.MustCompile(`[^a-z0-9]+`)
var logger = zap.NewNop()

type config struct {
	sourceDir   string
	mealieURL   string
	token       string
	dryRun      bool
	debug       bool
	uploadImage bool
	limit       int
}

type dryRunRecipe struct {
	SourcePath  string        `json:"sourcePath"`
	ImageUpload imagePreview  `json:"imageUpload"`
	Recipe      mealie.Recipe `json:"recipe"`
}

type imagePreview struct {
	Enabled       bool   `json:"enabled"`
	WillUpload    bool   `json:"willUpload"`
	ImageCount    int    `json:"imageCount"`
	MediaType     string `json:"mediaType,omitempty"`
	Extension     string `json:"extension,omitempty"`
	SizeBytes     int    `json:"sizeBytes,omitempty"`
	ConvertedFrom string `json:"convertedFrom,omitempty"`
	SkippedReason string `json:"skippedReason,omitempty"`
}

func Execute(ctx context.Context) error {
	return newRootCommand().ExecuteContext(ctx)
}

func newRootCommand() *cobra.Command {
	cfg := config{
		mealieURL:   os.Getenv("MEALIE_URL"),
		token:       os.Getenv("MEALIE_TOKEN"),
		uploadImage: true,
	}

	cmd := &cobra.Command{
		Use:   "mealie-importer <Recipes.melarecipes|recipe-directory>",
		Short: "Import a Mela .melarecipes export into Mealie",
		Long: "Import recipes from a Mela .melarecipes export bundle into Mealie.\n\n" +
			"The source can also be an extracted directory containing .melarecipe files.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.sourceDir = args[0]
			cfg.mealieURL = strings.TrimRight(strings.TrimSpace(cfg.mealieURL), "/")
			cfg.token = strings.TrimSpace(cfg.token)
			return run(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.mealieURL, "mealie-url", cfg.mealieURL, "Mealie base URL, or MEALIE_URL")
	cmd.Flags().StringVar(&cfg.token, "token", cfg.token, "Mealie API token, or MEALIE_TOKEN")
	cmd.Flags().BoolVar(&cfg.dryRun, "dry-run", false, "print import preview JSON without sending it")
	cmd.Flags().BoolVar(&cfg.debug, "debug", false, "enable debug logging to stderr")
	cmd.Flags().BoolVar(&cfg.uploadImage, "upload-image", true, "upload the first Mela image after creating each recipe")
	cmd.Flags().IntVar(&cfg.limit, "limit", 0, "maximum number of recipes to process")

	return cmd
}

func run(ctx context.Context, cfg config) error {
	runLogger, err := newLogger(cfg.debug)
	if err != nil {
		return err
	}
	logger = runLogger
	defer func() {
		_ = logger.Sync()
		logger = zap.NewNop()
	}()

	logger.Debug("starting import", zap.String("source", cfg.sourceDir), zap.Bool("dryRun", cfg.dryRun), zap.Bool("uploadImage", cfg.uploadImage), zap.Int("limit", cfg.limit))
	recipes, cleanup, err := mela.ReadSource(cfg.sourceDir)
	if err != nil {
		return err
	}
	defer cleanup()

	if cfg.limit > 0 && cfg.limit < len(recipes) {
		recipes = recipes[:cfg.limit]
	}
	logger.Debug("loaded recipes", zap.Int("count", len(recipes)))
	if len(recipes) == 0 {
		return fmt.Errorf("no .melarecipe files found in %s", cfg.sourceDir)
	}

	if cfg.dryRun {
		return printDryRun(ctx, recipes, cfg.uploadImage)
	}

	client, err := mealie.NewClient(cfg.mealieURL, cfg.token, mealie.WithLogger(logger.Named("mealie")))
	if err != nil {
		return err
	}
	categoryIndex, err := loadCategoryIndex(ctx, client, recipes)
	if err != nil {
		return err
	}

	for i, recipe := range recipes {
		converted := importer.Convert(recipe)
		if err := resolveRecipeCategories(&converted, categoryIndex); err != nil {
			return fmt.Errorf("%s: resolve categories: %w", recipe.Path, err)
		}
		logger.Debug("processing recipe", zap.Int("index", i+1), zap.Int("total", len(recipes)), zap.String("path", recipe.Path), zap.String("name", converted.Name), zap.Int("ingredients", len(converted.RecipeIngredient)), zap.Int("steps", len(converted.RecipeInstructions)), zap.Int("notes", len(converted.Notes)), zap.Int("images", len(recipe.Images)))

		slug, created, err := upsertRecipe(ctx, client, converted)
		if err != nil {
			return fmt.Errorf("%s: upsert recipe: %w", recipe.Path, err)
		}
		if created {
			fmt.Printf("[%d/%d] created %q\n", i+1, len(recipes), converted.Name)
		} else {
			fmt.Printf("[%d/%d] updated %q\n", i+1, len(recipes), converted.Name)
		}

		if cfg.uploadImage {
			image, ok, err := prepareImage(ctx, recipe)
			if err != nil {
				return err
			}
			if ok {
				logger.Debug("prepared recipe image", zap.String("slug", slug), zap.String("mediaType", image.MediaType), zap.String("extension", image.Extension), zap.String("convertedFrom", image.ConvertedFrom), zap.Int("bytes", len(image.Data)))
				if err := client.UploadRecipeImage(ctx, slug, image.Data, image.Extension); err != nil {
					return fmt.Errorf("%s: upload image for %q: %w", recipe.Path, slug, err)
				}
			} else {
				logger.Debug("no recipe image to upload", zap.String("slug", slug))
			}
		}
	}

	return nil
}

func loadCategoryIndex(ctx context.Context, client *mealie.Client, recipes []mela.Recipe) (map[string]mealie.Organizer, error) {
	if !usesCategories(recipes) {
		return nil, nil
	}

	categories, err := client.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list mealie categories: %w", err)
	}

	index := make(map[string]mealie.Organizer, len(categories))
	for _, category := range categories {
		key := categoryKey(category.Name)
		if key == "" {
			continue
		}
		if existing, ok := index[key]; ok && existing.ID != category.ID {
			return nil, fmt.Errorf("multiple mealie categories named %q", category.Name)
		}
		index[key] = category
	}
	return index, nil
}

func usesCategories(recipes []mela.Recipe) bool {
	for _, recipe := range recipes {
		for _, category := range recipe.Categories {
			if strings.TrimSpace(category) != "" {
				return true
			}
		}
	}
	return false
}

func resolveRecipeCategories(recipe *mealie.Recipe, categories map[string]mealie.Organizer) error {
	if len(recipe.RecipeCategory) == 0 {
		return nil
	}
	if categories == nil {
		return fmt.Errorf("recipe has categories but mealie categories were not loaded")
	}

	resolved := make([]mealie.Organizer, 0, len(recipe.RecipeCategory))
	seen := make(map[string]struct{}, len(recipe.RecipeCategory))
	for _, category := range recipe.RecipeCategory {
		key := categoryKey(category.Name)
		existing, ok := categories[key]
		if !ok || existing.ID == "" {
			return fmt.Errorf("category %q does not exist in mealie", category.Name)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		resolved = append(resolved, existing)
	}
	recipe.RecipeCategory = resolved
	return nil
}

func categoryKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func upsertRecipe(ctx context.Context, client *mealie.Client, recipe mealie.Recipe) (string, bool, error) {
	logger.Debug("upserting recipe", zap.String("name", recipe.Name), zap.String("expectedSlug", recipeSlug(recipe.Name)))
	existing, found, err := findExistingRecipe(ctx, client, recipe.Name)
	if err != nil {
		return "", false, err
	}
	if found {
		logger.Debug("upsert found existing recipe", zap.String("name", recipe.Name), zap.String("slug", existing.Slug))
		if existing.Slug == "" {
			return "", false, fmt.Errorf("existing recipe %q has no slug", recipe.Name)
		}
		if err := updateRecipe(ctx, client, existing.Slug, recipe); err != nil {
			return "", false, err
		}
		return existing.Slug, false, nil
	}

	slug, err := client.CreateRecipe(ctx, recipe.Name)
	if err != nil {
		if mealie.IsAlreadyExists(err) {
			logger.Debug("create reported recipe already exists; retrying lookup", zap.String("name", recipe.Name), zap.Error(err))
			return updateExistingAfterCreateConflict(ctx, client, recipe)
		}
		return "", false, err
	}
	logger.Debug("upsert created stub", zap.String("name", recipe.Name), zap.String("slug", slug))
	if err := updateRecipe(ctx, client, slug, recipe); err != nil {
		return "", false, fmt.Errorf("update newly created recipe %q: %w", slug, err)
	}
	return slug, true, nil
}

func updateExistingAfterCreateConflict(ctx context.Context, client *mealie.Client, recipe mealie.Recipe) (string, bool, error) {
	existing, found, err := findExistingRecipe(ctx, client, recipe.Name)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, fmt.Errorf("recipe %q already exists, but lookup by name or slug could not find it", recipe.Name)
	}
	if existing.Slug == "" {
		return "", false, fmt.Errorf("existing recipe %q has no slug", recipe.Name)
	}
	if err := updateRecipe(ctx, client, existing.Slug, recipe); err != nil {
		return "", false, fmt.Errorf("update existing recipe %q after create conflict: %w", existing.Slug, err)
	}
	return existing.Slug, false, nil
}

func updateRecipe(ctx context.Context, client *mealie.Client, slug string, recipe mealie.Recipe) error {
	logger.Debug("loading current recipe before update", zap.String("slug", slug), zap.String("incomingName", recipe.Name), zap.String("incomingSlug", recipe.Slug))
	current, found, err := client.GetRecipe(ctx, slug)
	if err != nil {
		return fmt.Errorf("get existing recipe %q before update: %w", slug, err)
	}
	if !found {
		return fmt.Errorf("recipe %q was not found before update", slug)
	}

	recipe.ID = current.ID
	recipe.UserID = current.UserID
	recipe.HouseholdID = current.HouseholdID
	recipe.GroupID = current.GroupID
	recipe.Slug = current.Slug
	if recipe.Slug == "" {
		recipe.Slug = slug
	}

	logger.Debug("sending recipe update", zap.String("slug", slug), zap.String("id", recipe.ID), zap.String("name", recipe.Name), zap.String("payloadSlug", recipe.Slug), zap.String("userId", recipe.UserID), zap.String("householdId", recipe.HouseholdID), zap.String("groupId", recipe.GroupID), zap.Int("ingredients", len(recipe.RecipeIngredient)), zap.Int("steps", len(recipe.RecipeInstructions)), zap.Int("notes", len(recipe.Notes)))

	return client.UpdateRecipe(ctx, slug, recipe)
}

func findExistingRecipe(ctx context.Context, client *mealie.Client, name string) (mealie.RecipeSummary, bool, error) {
	logger.Debug("looking for existing recipe", zap.String("name", name))
	existing, found, err := client.FindRecipeByName(ctx, name)
	if err != nil || found {
		return existing, found, err
	}

	slug := recipeSlug(name)
	if slug == "" {
		return mealie.RecipeSummary{}, false, nil
	}
	logger.Debug("name lookup missed; checking expected slug", zap.String("name", name), zap.String("slug", slug))
	return client.FindRecipeBySlug(ctx, slug)
}

func newLogger(debug bool) (*zap.Logger, error) {
	if !debug {
		return zap.NewNop(), nil
	}
	cfg := zap.NewDevelopmentConfig()
	cfg.DisableStacktrace = true
	return cfg.Build()
}

func recipeSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = slugCleanup.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}

func printDryRun(ctx context.Context, recipes []mela.Recipe, uploadImage bool) error {
	output := make([]dryRunRecipe, 0, len(recipes))
	for _, recipe := range recipes {
		image, err := previewImage(ctx, recipe, uploadImage)
		if err != nil {
			return err
		}
		output = append(output, dryRunRecipe{
			SourcePath:  recipe.Path,
			ImageUpload: image,
			Recipe:      importer.Convert(recipe),
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func previewImage(ctx context.Context, recipe mela.Recipe, uploadImage bool) (imagePreview, error) {
	preview := imagePreview{
		Enabled:    uploadImage,
		ImageCount: len(recipe.Images),
	}
	if !uploadImage {
		preview.SkippedReason = "image upload disabled"
		return preview, nil
	}

	image, ok, err := prepareImage(ctx, recipe)
	if err != nil {
		return imagePreview{}, err
	}
	if !ok {
		preview.SkippedReason = "recipe has no image"
		return preview, nil
	}

	preview.WillUpload = true
	preview.MediaType = image.MediaType
	preview.Extension = image.Extension
	preview.SizeBytes = len(image.Data)
	preview.ConvertedFrom = image.ConvertedFrom
	return preview, nil
}

func prepareImage(ctx context.Context, recipe mela.Recipe) (mela.Image, bool, error) {
	image, ok, err := recipe.PrimaryImage()
	if err != nil || !ok {
		return image, ok, err
	}

	if !imageconv.NeedsHEIFConversion(image.Extension) {
		return image, true, nil
	}

	converted, err := imageconv.HEIFToJPEG(ctx, image.Data)
	if err != nil {
		return mela.Image{}, false, err
	}

	return mela.Image{
		Data:          converted,
		MediaType:     "image/jpeg",
		Extension:     "jpg",
		ConvertedFrom: image.Extension,
	}, true, nil
}
