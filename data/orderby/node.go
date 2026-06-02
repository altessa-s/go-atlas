// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby

import (
	"fmt"
	"slices"
)

// Direction is the sort direction for a single AIP-132 order_by key.
type Direction uint8

const (
	// DirectionAscending is the default direction. Equivalent to the
	// explicit `asc` token in the DSL.
	DirectionAscending Direction = iota
	// DirectionDescending is the direction selected by an explicit `desc`
	// token in the DSL.
	DirectionDescending
)

// String returns the AIP-132 DSL token for the direction ("asc" or
// "desc"). The output mirrors the wire-level grammar so logs and
// fmt-formatted values stay close to what callers wrote in the request.
// Unknown values fall back to a Stringer-style placeholder.
func (d Direction) String() string {
	switch d {
	case DirectionAscending:
		return "asc"
	case DirectionDescending:
		return "desc"
	default:
		return fmt.Sprintf("Direction(%d)", uint8(d))
	}
}

// Key is a single (field path, direction) sort key.
//
// Name is the canonical dotted form ("address.city") and matches the
// AIP-132 wire form one-to-one. Callers that need segment-level access
// can `strings.Split(k.Name, ".")` — the parser does the same split for
// validation but discards the result so it does not get carried through
// the LRU cache on every hit.
type Key struct {
	Name      string
	Direction Direction
}

// IsDescending reports whether the key is sorted in descending order.
func (k Key) IsDescending() bool { return k.Direction == DirectionDescending }

// Spec is the parsed result of an AIP-132 order_by string. The Keys are
// kept in the same order they appeared in the source so translators can
// emit storage-specific sort fragments that respect precedence.
//
// Naming note: the AIP-132 wire field is `order_by`. The obvious type
// name `OrderBy` stutters with the package name, so the parsed value is
// named Spec — read as "the parsed order_by specification". The package
// name still reflects the wire concept for discoverability.
type Spec struct {
	Keys []Key
}

// IsEmpty reports whether the Spec has no sort keys, i.e. the input was
// empty or whitespace-only.
func (s Spec) IsEmpty() bool { return len(s.Keys) == 0 }

// Clone returns a copy of s whose Keys slice is independent of the
// original. Mutating the result cannot affect the original — in
// particular, [Parser.Parse] uses Clone to hand callers a value that is
// safe to mutate even when it came from the parser's LRU cache. Key
// fields are value types (string and uint8) so a shallow copy of the
// slice is enough.
func (s Spec) Clone() Spec {
	if len(s.Keys) == 0 {
		return Spec{}
	}
	return Spec{Keys: slices.Clone(s.Keys)}
}
