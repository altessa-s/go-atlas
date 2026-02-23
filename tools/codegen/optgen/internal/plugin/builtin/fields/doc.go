// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package fields provides field-handler plugins that generate With* (or
// Without*) functional-option functions for each struct field.
//
// Plugin selection happens by priority: for a given field the generator picks
// the highest-priority plugin whose CanHandle returns true. Lower-priority
// plugins act as fallbacks.
//
// Built-in handlers (highest priority first):
//
//   - [ManualPlugin] (110) -- emits nothing; used for opt:"-" with default or optgen:"manual"
//   - [NetIPParsePlugin] (40) -- parses string arguments into []net.IP
//   - [SliceSetPlugin] (35) -- replaces a slice wholesale (default for slices)
//   - [BoolFlagPlugin] (20) -- zero-argument With/Without toggle for bool fields
//   - [DefaultSetterPlugin] (0) -- scalar assignment; for string fields generates a
//     generic T interface{string|*string} overload with transform chain
//   - [DefaultAppenderPlugin] (0) -- variadic append for slices when optgen:"append" is set
package fields
