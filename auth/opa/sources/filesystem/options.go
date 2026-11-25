// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filesystem

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
)

// DefaultExtensions are the default file extensions to include.
var DefaultExtensions = []string{".rego"}

// options holds the configuration for the filesystem source.
type options struct {
	// extensions sets the file extensions to include when loading policies.
	// Defaults to [".rego"].
	extensions []string `optgen:"default=DefaultExtensions"`
	// includeData enables loading .json files as OPA data.
	// When enabled, JSON files in the policy directory are loaded into OPA's data store.
	includeData bool
	// logger sets the logger for the filesystem source.
	logger *slog.Logger
	// checksums maps relative policy file paths to their expected SHA-256 hex digests.
	// When non-nil, every loaded file must match its expected hash and unknown files are rejected.
	checksums map[string]string `optgen:"default=nil"`
	// WithReleaseManifest sets the path relative to the bundle directory (or the single policy file's parent).
	// The manifest is read once per fetch. Use either this option or WithChecksums.
	releaseManifest string
}
