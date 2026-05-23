package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/victor/mealie-importer/internal/importer"
	"github.com/victor/mealie-importer/internal/mealie"
	"github.com/victor/mealie-importer/internal/mela"
)

type config struct {
	sourceDir   string
	mealieURL   string
	token       string
	dryRun      bool
	uploadImage bool
	limit       int
}

func main() {
	if err := newRootCommand().ExecuteContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
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
	cmd.Flags().BoolVar(&cfg.dryRun, "dry-run", false, "print converted Mealie JSON without sending it")
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
		return printDryRun(recipes)
	}

	client, err := mealie.NewClient(cfg.mealieURL, cfg.token)
	if err != nil {
		return err
	}

	for i, recipe := range recipes {
		converted := importer.Convert(recipe)
		fmt.Printf("[%d/%d] creating %q\n", i+1, len(recipes), converted.Name)

		slug, err := client.CreateRecipe(ctx, converted.Name)
		if err != nil {
			return fmt.Errorf("%s: create recipe: %w", recipe.Path, err)
		}
		if err := client.UpdateRecipe(ctx, slug, converted); err != nil {
			return fmt.Errorf("%s: update recipe %q: %w", recipe.Path, slug, err)
		}

		if cfg.uploadImage {
			image, ok, err := recipe.PrimaryImage()
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

func printDryRun(recipes []mela.Recipe) error {
	output := make([]mealie.Recipe, 0, len(recipes))
	for _, recipe := range recipes {
		output = append(output, importer.Convert(recipe))
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
