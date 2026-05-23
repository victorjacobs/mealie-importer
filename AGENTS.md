# AGENTS.md

## Project

This repository contains `mealie-importer`, a Go project for importing Mela recipe exports into the Mealie hosted recipe manager.

The importer should preserve recipe data as faithfully as practical while adapting it to Mealie's API and data model. Prefer clear, testable conversion code over implicit behavior.

## Environment

Work inside the direnv-managed Nix environment for this repository.

Before running project commands, ensure direnv has loaded the flake environment:

```sh
direnv allow
direnv reload
```

If direnv is not hooked into the current shell, run commands through the flake explicitly:

```sh
nix develop -c <command>
```

Do not assume a globally installed Go toolchain. The repository flake provides the expected Go version.

## Development Notes

- Use the Go toolchain from the Nix flake.
- Keep importer behavior deterministic and easy to test.
- Add focused tests for parsing, conversion, and Mealie API payload generation as those areas are implemented.
- Prefer standard library packages unless a dependency materially simplifies archive parsing, API interaction, or structured data handling.
- Avoid committing generated files, local credentials, API tokens, or personal recipe exports.

## Mealie Integration

- Treat the Mealie API as the source of truth for supported import payloads and authentication behavior.
- Keep API base URLs, credentials, and tokens configurable through environment variables or configuration files that are safe to exclude from Git.
- Do not hard-code personal hosted Mealie instance details.

## Mela Import Behavior

- Preserve recipe titles, descriptions, ingredients, instructions, notes, images, categories, tags, source URLs, yield, prep time, cook time, and other metadata where available.
- When Mela fields do not map cleanly to Mealie fields, prefer explicit conversion logic and tests documenting the chosen behavior.
- Handle malformed or incomplete recipe data gracefully and report actionable errors.
