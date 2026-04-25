# goconfig

CLI tool for converting configuration files between formats. Supports YAML, TOML, `.env`, and Markdown with auto-detection from file
extensions, directory merging, comment handling, and optional AI-powered description generation via the Claude API.

## Usage

```
goconfig convert --from config.yaml --to .env
goconfig convert --from ./configs/ --to merged.env
goconfig convert --from config.yaml --to docs.md --use-claude
```

## Supported conversions

| From         | To           | Description                                                                       |
|--------------|--------------|-----------------------------------------------------------------------------------|
| YAML / TOML  | `.env`       | Flattens nested structures using `__` delimiter and `SCREAMING_SNAKE_CASE`        |
| `.env`       | YAML / TOML  | Reconstructs nested structures with automatic type inference                      |
| YAML         | TOML         | Direct format conversion preserving structure                                     |
| TOML         | YAML         | Direct format conversion preserving structure                                     |
| YAML / TOML  | Markdown     | Generates reference tables, optionally enriched with Claude-generated descriptions|

## Flags

| Flag               | Description                                                                            |
|--------------------|----------------------------------------------------------------------------------------|
| `--from`, `-f`     | Source file or directory path (required)                                                |
| `--to`, `-t`       | Target file path (required)                                                            |
| `--from-format`    | Explicit source format when extension is non-standard                                  |
| `--to-format`      | Explicit target format when extension is non-standard                                  |
| `--parse-comments` | Include commented lines in conversion output                                           |
| `--use-claude`     | Use Claude API for AI-generated field descriptions in Markdown output                  |
| `--claude-api-key` | Claude API key (or set `ANTHROPIC_API_KEY` environment variable)                       |

## Subpackages

| Package                  | Description                                                                        |
|--------------------------|------------------------------------------------------------------------------------|
| [commands](./commands)   | Cobra root command with the `convert` subcommand                                   |
