// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validators

import "slices"

// BackendTypes supplies application-registered names to a schema's selector rule.
// Its zero value admits only the built-ins passed to AllowedTypes.
type BackendTypes[T ~string] struct {
	// A pointer to an immutable snapshot keeps the containing schema comparable.
	registered *[]T
}

// SetRegisteredTypes replaces the additional names admitted during validation.
// Call it before loading or validating a configuration that uses custom backends.
func (b *BackendTypes[T]) SetRegisteredTypes(names []T) {
	b.registered = nil
	if len(names) > 0 {
		b.registered = new(slices.Clone(names))
	}
}

// AllowedTypes includes the built-in names and the configured registration snapshot.
func (b BackendTypes[T]) AllowedTypes(builtins ...T) []T {
	if b.registered == nil {
		return builtins
	}
	return slices.Concat(builtins, *b.registered)
}
