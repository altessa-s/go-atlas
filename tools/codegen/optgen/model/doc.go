// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package model defines the public data structures shared between optgen's
// parser, generator, and plugin subsystems.
//
// [OptField] is the central type: it captures everything the parser extracts
// from a single struct field (name, type, tags, modifiers, checks, metadata).
// Plugins receive OptField values and return generated code.
//
// [GenericInfo] carries type parameter information for generic option structs,
// enabling the generator to produce parameterised WithXxx functions.
//
// External plugins should import this package rather than any internal package.
package model
