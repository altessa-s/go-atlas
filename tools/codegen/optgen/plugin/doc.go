// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package plugin defines the public extension API for the optgen code generator.
//
// This package lives outside /internal so that external .so plugins and
// third-party libraries can implement optgen extensions. The key abstractions are:
//
//   - [FieldPlugin] -- selects and generates the WithXxx function for a field.
//   - [TransformPlugin] -- applies a single optval modifier (e.g. "lower", "trimspaces").
//   - [ModifierPlugin] -- unified pipeline modifier for guards, post-processing, and checks.
//   - [TypeDefaultPlugin] -- supplies a default-value expression for a field type.
//   - [CheckPlugin] -- generates validation code for an optcheck key.
//
// All plugin types carry [Meta] metadata (kind, priority, applicable types) via
// the [PluginMeta] interface.
//
// # Registration
//
// Plugins register themselves in init() via the global [Register] function,
// which dispatches to the appropriate [Registry] method by type-assertion:
//
//	func init() {
//		plugin.Register(&MyFieldPlugin{}, &MyDefaults{}, &MyCheck{})
//	}
//
// # Concurrency
//
// The global [Registry] is protected by a sync.RWMutex and safe for concurrent
// reads. Registration is expected to happen only during init().
//
// # Pipeline Execution
//
// For each field, the generator calls [BuildPipeline] which runs collected
// modifiers in phase order: guard, transform, post-process, check. See [Phase]
// constants for the ordering.
package plugin
