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

Useful flags:

- `--source`: directory containing `.melarecipe` files, alternative to the positional path
- `--dry-run`: print import preview JSON, including image upload status, and do not call the API
- `--limit`: process only the first N recipes
- `--mealie-url`: Mealie base URL, alternative to `MEALIE_URL`
- `--token`: Mealie API token, alternative to `MEALIE_TOKEN`
- `--upload-image=false`: skip primary image upload

HEIC/HEIF images are converted to JPEG before upload using `heif-dec` from the Nix development environment. The dry-run output reports these as `extension: "jpg"` with `convertedFrom: "heic"` or `convertedFrom: "heif"`.

## Current Import Strategy

For each Mela recipe, the importer:

1. Creates a Mealie recipe with `POST /api/recipes`.
2. Updates the created recipe with converted fields using `PUT /api/recipes/{slug}`.
3. Converts HEIC/HEIF images to JPEG when needed.
4. Uploads the first embedded Mela image with `PUT /api/recipes/{slug}/image`.

Mela notes and nutrition text are preserved as Mealie notes. Mela IDs, favorite state, and want-to-cook state are preserved in recipe extras.
