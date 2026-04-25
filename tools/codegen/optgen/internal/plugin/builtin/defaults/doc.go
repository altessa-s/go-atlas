// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package defaults provides type-default plugins that supply initial values
// for struct fields when the user has not specified an explicit default in the
// optgen tag.
//
// Each plugin implements [plugin.TypeDefaultPlugin] and is registered via init.
// When the parser encounters a field with no "default=" in its optgen tag, the
// generator queries registered type-default plugins in priority order.
//
// Built-in defaults:
//
//   - [LoggerDefaultPlugin] -- *slog.Logger -> slog.New(slog.DiscardHandler)
//   - [MapDefaultPlugin] -- map[K]V -> make(map[K]V)
//   - [SerializerDefaultPlugin] -- serializer.Serializer -> &serializer.JSON{}
package defaults
