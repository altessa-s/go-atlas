// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package modifiers provides value transformation functions for the
// [normalizer] package.
//
// Built-in modifiers are registered at init time:
//
//   - "trim" — trims leading/trailing whitespace
//   - "lowercase" — converts to lowercase
//   - "uppercase" — converts to uppercase
//   - "nil_on_empty" — sets *string fields to nil when empty
//   - "phone" — normalizes phone numbers to E.164 via [NormalizePhone]
//   - "remove_bad_symbols" — strips control chars and deprecated Unicode
//   - "remove_empty_elements" — removes empty/nil entries from string slices
//
// Custom modifiers can be added with [RegisterModifier] and retrieved with
// [GetModifier]. Both functions are safe for concurrent use.
//
// Example:
//
//	modifiers.RegisterModifier("custom", CustomModifier)
//	mod, ok := modifiers.GetModifier("trim")
package modifiers
