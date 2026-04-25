// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// This file defines the CheckPlugin interface and global registry functions for
// optcheck validation plugins. CheckPlugins generate validation code for field
// values based on optcheck tag annotations (e.g., "required", "minlen=3").

package plugin

import "github.com/altessa-s/go-atlas/tools/codegen/optgen/model"

// CheckSpec describes a registered optcheck key, including whether the check
// requires a value (e.g. "minlen=3" vs bare "required") and its execution
// priority relative to other checks.
type CheckSpec struct {
	Key           string
	RequiresValue bool
	// Priority controls the ordering of generated checks (lower first).
	// Use small integers (10, 20, 30) to leave gaps for custom checks.
	Priority int
}

// CheckPlugin generates validation code for a single optcheck key
// (e.g. "required", "minlen", "oneof"). Use [CheckBase] as an embedding
// helper to supply default Spec and Aliases implementations.
type CheckPlugin interface {
	Spec() CheckSpec
	Aliases() []string

	// Generate returns code lines (without indentation) to be inserted inside the option body.
	// Plugins may call ctx.AddImport(...) if needed.
	Generate(ctx GenerationContext, field model.OptField, kind string, valueVar, rawValue string) []string
}

// CheckSpecs returns known optcheck specs from the global registry.
func CheckSpecs() map[string]CheckSpec { return defaultRegistry.CheckSpecs() }

// FindCheckPlugin finds an optcheck plugin by key (or alias) in the global registry.
func FindCheckPlugin(key string) (CheckPlugin, CheckSpec, bool) {
	return defaultRegistry.FindCheckPlugin(key)
}

// KnownCheckKeys returns all registered optcheck keys (including aliases) in the global registry.
func KnownCheckKeys() []string { return defaultRegistry.KnownCheckKeys() }
