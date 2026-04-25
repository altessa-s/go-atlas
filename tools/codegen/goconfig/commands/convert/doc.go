// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package convert implements the "goconfig convert" subcommand and the
// converters that transform configuration files between YAML, TOML, .env,
// and Markdown formats.
//
// # Naming conventions
//
// Environment variable keys follow the go-tools config parser conventions:
//
//   - Nested structures are delimited by __ (double underscore).
//   - camelCase field names become SCREAMING_SNAKE_CASE.
//   - Example: grpc.interceptors.realIp becomes GRPC__INTERCEPTORS__REAL_IP.
//
// # Converters
//
//   - [ConfigToEnvConverter] -- YAML/TOML to .env, with optional comment tracking.
//   - [EnvToConfigConverter] -- .env to YAML/TOML, with automatic type inference.
//   - [ConfigToConfigConverter] -- YAML to TOML or TOML to YAML.
//   - [ConfigToMarkdownConverter] -- YAML/TOML to Markdown reference tables,
//     optionally enriched with AI-generated descriptions via [ClaudeClient].
//
// # Comment handling
//
// When --parse-comments is set, fully-commented YAML template files are
// auto-uncommented before parsing so their values can be extracted.
// Inline YAML comments are tracked per-key and re-emitted as commented
// lines (# KEY=VALUE) in .env output.
package convert
