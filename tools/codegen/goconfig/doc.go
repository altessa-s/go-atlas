// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package main provides goconfig, a CLI tool for converting configuration
// files between formats.
//
// Supported formats are YAML, TOML, .env, and Markdown. The tool
// auto-detects formats from file extensions and can be overridden with
// explicit --from-format / --to-format flags.
//
// When the source path is a directory, all matching config files are loaded
// and deep-merged into a single output. Files below the
// [convert.parallelProcessingThreshold] are processed sequentially;
// larger batches are processed concurrently.
//
// Commented-out YAML blocks are handled specially: fully-commented template
// files are auto-uncommented so their values can be extracted, while
// mixed files preserve only real (uncommented) content unless
// --parse-comments is set.
//
// Single-file conversion:
//
//	goconfig convert --from config.yaml --to .env
//
// Directory merge:
//
//	goconfig convert --from ./configs/ --to merged.env
package main
