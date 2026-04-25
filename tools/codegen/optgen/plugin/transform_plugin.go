// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// This file provides global registry functions for TransformPlugin lookup.
// TransformPlugins apply optval-style modifiers (e.g., "lower", "trimspaces")
// to field expressions during code generation.

package plugin

// FindTransformPlugin finds a transform plugin by key in the global registry,
// applying disabled/type filtering.
func FindTransformPlugin(key, typeStr string) (TransformPlugin, bool) {
	return defaultRegistry.FindTransformPlugin(key, typeStr)
}
