package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/victorjacobs/mealie-importer/internal/imageconv"
	"github.com/victorjacobs/mealie-importer/internal/importer"
	"github.com/victorjacobs/mealie-importer/internal/mealie"
	"github.com/victorjacobs/mealie-importer/internal/mela"
)

type config struct {
	sourceDir   string
	mealieURL   string
	token       string
	dryRun      bool
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
		Use:   "mealie-importer [path]",
		Short: "Import Mela recipe exports into Mealie",
		Args: func(cmd *cobra.Command, args []string) error {
			if cfg.sourceDir == "" && len(args) > 0 {
				cfg.sourceDir = args[0]
			}
			if cfg.sourceDir == "" {
				return fmt.Errorf("source directory is required")
			}
			if len(args) > 1 {
				return fmt.Errorf("expected at most one source directory argument")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.mealieURL = strings.TrimRight(strings.TrimSpace(cfg.mealieURL), "/")
			cfg.token = strings.TrimSpace(cfg.token)
			return run(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.sourceDir, "source", "", "directory containing .melarecipe files")
	cmd.Flags().StringVar(&cfg.mealieURL, "mealie-url", cfg.mealieURL, "Mealie base URL, or MEALIE_URL")
	cmd.Flags().StringVar(&cfg.token, "token", cfg.token, "Mealie API token, or MEALIE_TOKEN")
	cmd.Flags().BoolVar(&cfg.dryRun, "dry-run", false, "print import preview JSON without sending it")
	cmd.Flags().BoolVar(&cfg.uploadImage, "upload-image", true, "upload the first Mela image after creating each recipe")
	cmd.Flags().IntVar(&cfg.limit, "limit", 0, "maximum number of recipes to process")

	return cmd
}

func run(ctx context.Context, cfg config) error {
	recipes, err := mela.ReadDir(cfg.sourceDir)
	if err != nil {
		return err
	}
	if cfg.limit > 0 && cfg.limit < len(recipes) {
		recipes = recipes[:cfg.limit]
	}
	if len(recipes) == 0 {
		return fmt.Errorf("no .melarecipe files found in %s", cfg.sourceDir)
	}

	if cfg.dryRun {
		return printDryRun(ctx, recipes, cfg.uploadImage)
	}

	client, err := mealie.NewClient(cfg.mealieURL, cfg.token)
	if err != nil {
		return err
	}

	for i, recipe := range recipes {
		converted := importer.Convert(recipe)

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
				if err := client.UploadRecipeImage(ctx, slug, image.Data, image.Extension); err != nil {
					return fmt.Errorf("%s: upload image for %q: %w", recipe.Path, slug, err)
				}
			}
		}
	}

	return nil
}

func upsertRecipe(ctx context.Context, client *mealie.Client, recipe mealie.Recipe) (string, bool, error) {
	existing, found, err := client.FindRecipeByName(ctx, recipe.Name)
	if err != nil {
		return "", false, err
	}
	if found {
		if existing.Slug == "" {
			return "", false, fmt.Errorf("existing recipe %q has no slug", recipe.Name)
		}
		if err := client.UpdateRecipe(ctx, existing.Slug, recipe); err != nil {
			return "", false, err
		}
		return existing.Slug, false, nil
	}

	slug, err := client.CreateRecipe(ctx, recipe.Name)
	if err != nil {
		return "", false, err
	}
	if err := client.UpdateRecipe(ctx, slug, recipe); err != nil {
		return "", false, err
	}
	return slug, true, nil
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
