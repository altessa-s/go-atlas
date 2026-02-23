// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo

import (
	"fmt"
	"strings"
)

// parseSemVer parses a semantic version string.
// It supports basic SemVer 2.0.0 format: Major.Minor.Patch[-Prerelease][+Build]
// Note: This is a simplified internal implementation to avoid external dependencies.
func parseSemVer(v string) (*SemanticVersion, error) {
	if v == "" {
		return nil, fmt.Errorf("empty version string")
	}

	// Remove 'v' prefix if present
	v = strings.TrimPrefix(v, "v")

	// Split into version and build metadata
	parts := strings.SplitN(v, "+", 2)
	versionPart := parts[0]

	// Split into version and prerelease
	parts = strings.SplitN(versionPart, "-", 2)
	corePart := parts[0]
	var prerelease []string
	if len(parts) > 1 {
		prerelease = strings.Split(parts[1], ".")
	}

	// Split core part into major, minor, patch
	coreParts := strings.Split(corePart, ".")
	if len(coreParts) < 1 || len(coreParts) > 3 {
		return nil, fmt.Errorf("invalid version format: %s", v)
	}

	sv := &SemanticVersion{
		Major:      coreParts[0],
		Prerelease: prerelease,
	}

	if len(coreParts) > 1 {
		sv.Minor = coreParts[1]
	} else {
		sv.Minor = "0"
	}

	const patchIndex = 2
	if len(coreParts) > patchIndex {
		sv.Patch = coreParts[2]
	} else {
		sv.Patch = "0"
	}

	return sv, nil
}
