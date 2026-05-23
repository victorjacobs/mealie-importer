# mealie-importer

CLI tool for importing Mela `.melarecipe` exports into a hosted Mealie instance.

## Environment

Use the repository direnv environment:

```sh
direnv allow
direnv reload
```

If direnv is not active in your shell, prefix commands with `nix develop -c`.

## Usage

Run the Nix-packaged importer:

```sh
nix run . -- --dry-run /Users/victor/Downloads/Recipes.melarecipes
```

Preview the import plan without importing anything:

```sh
go run . --dry-run /Users/victor/Downloads/Recipes.melarecipes
```

Import into Mealie:

```sh
export MEALIE_URL="https://mealie.example.com"
export MEALIE_TOKEN="your-api-token"

go run . /Users/victor/Downloads/Recipes.melarecipes
```

Build a wrapped binary:

```sh
nix build .#mealie-importer
./result/bin/mealie-importer --dry-run /Users/victor/Downloads/Recipes.melarecipes
```

Useful flags:

- `--source`: directory containing `.melarecipe` files, alternative to the positional path
- `--dry-run`: print import preview JSON, including image upload status, and do not call the API
- `--limit`: process only the first N recipes
- `--mealie-url`: Mealie base URL, alternative to `MEALIE_URL`
- `--token`: Mealie API token, alternative to `MEALIE_TOKEN`
- `--upload-image=false`: skip primary image upload

HEIC/HEIF images are converted to JPEG before upload using `heif-dec`. The Nix-packaged binary is wrapped so it can find `heif-dec` at runtime. When using `go run .`, use the direnv/Nix development environment so `heif-dec` is available on `PATH`. The dry-run output reports converted images as `extension: "jpg"` with `convertedFrom: "heic"` or `convertedFrom: "heif"`.

## Current Import Strategy

For each Mela recipe, the importer:

1. Searches Mealie for an existing recipe with the same name.
2. Creates a Mealie recipe with `POST /api/recipes` only when no exact name match exists.
3. Updates the existing or created recipe with converted fields using `PUT /api/recipes/{slug}`.
4. Converts HEIC/HEIF images to JPEG when needed.
5. Uploads the first embedded Mela image with `PUT /api/recipes/{slug}/image`.

Duplicate detection first uses an exact, case-insensitive recipe name match in Mealie search results. If search misses a recipe that already exists, the importer also checks the expected Mealie slug and can recover an empty stub created by an earlier failed run.

Mela notes and nutrition text are preserved as Mealie notes. Mela IDs, favorite state, and want-to-cook state are preserved in recipe extras.
