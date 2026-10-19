// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package enumcodec

import (
	"fmt"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Codec maps a proto enum P to a domain enum D and back. The proto zero value
// (UNSPECIFIED) maps to the zero D, so a partial update can leave an enum
// unset; domain validation rejects it where it is required. A Codec is
// immutable and safe for concurrent use; the zero Codec maps nothing.
type Codec[P ~int32, D comparable] struct {
	toDomain *coremaps.ImmutableMap[P, D]
	toProto  *coremaps.ImmutableMap[D, P]
}

// New returns the Codec of the one-to-one mapping m from defined non-zero
// proto values to non-zero domain values; m is copied. It panics when m maps
// the proto zero value (UNSPECIFIED always decodes to the zero D), maps a
// value to the zero D (indistinguishable from unset) or maps two proto
// values to one domain value (the reverse direction would be ambiguous):
// these are programming errors in a static table.
func New[P ~int32, D comparable](m map[P]D) Codec[P, D] {
	var zeroD D
	toDomain := make(map[P]D, len(m))
	toProto := make(map[D]P, len(m))
	for p, d := range m {
		switch {
		case p == 0:
			panic(fmt.Sprintf("enumcodec: the proto zero value is mapped to %v", d))
		case d == zeroD:
			panic(fmt.Sprintf("enumcodec: proto value %d is mapped to the zero domain value", p))
		}
		if prev, dup := toProto[d]; dup {
			panic(fmt.Sprintf("enumcodec: proto values %d and %d are both mapped to %v", min(prev, p), max(prev, p), d))
		}
		toDomain[p] = d
		toProto[d] = p
	}
	return Codec[P, D]{toDomain: coremaps.NewImmutableMap(toDomain), toProto: coremaps.NewImmutableMap(toProto)}
}

// Domain decodes p received at the API field path field. The zero value
// (UNSPECIFIED) is the zero D and no error; an undefined non-zero value is a
// core/errors.FieldError on field, "unknown value <p>".
func (c Codec[P, D]) Domain(field string, p P) (D, error) {
	var zero D
	if p == 0 {
		return zero, nil
	}
	var (
		d  D
		ok bool
	)
	if c.toDomain != nil {
		d, ok = c.toDomain.Get(p)
	}
	if !ok {
		return zero, coreerrs.NewFieldError(field, fmt.Sprintf("unknown value %d", p))
	}
	return d, nil
}

// Proto encodes d; the zero D or an unknown domain value encodes as the
// proto zero value (UNSPECIFIED).
func (c Codec[P, D]) Proto(d D) P {
	if c.toProto == nil {
		return 0
	}
	p, _ := c.toProto.Get(d)
	return p
}
