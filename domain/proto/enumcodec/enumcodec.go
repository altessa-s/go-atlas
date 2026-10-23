// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package enumcodec

import (
	"errors"
	"fmt"
	"reflect"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
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

// ProtoE encodes d, returning a FieldError with an empty path for an unknown
// non-zero value. The zero D encodes as UNSPECIFIED without error.
func (c Codec[P, D]) ProtoE(d D) (P, error) {
	var zero D
	if d == zero {
		return 0, nil
	}
	if c.toProto != nil {
		if p, ok := c.toProto.Get(d); ok {
			return p, nil
		}
	}
	return 0, coreerrs.NewFieldError("", fmt.Sprintf("unknown value %v", d))
}

// ErrorCodec adapts the mapping to converter.WithErrorCodecs. Unknown values
// produce FieldErrors on the full source path. Exact P/D and *P/*D pairs
// are supported; nil pointers clear the destination, and non-nil pointers
// produce newly allocated destination pointers.
func (c Codec[P, D]) ErrorCodec() convcodec.CodecE {
	return func(path string, src, dst reflect.Value, next convcodec.CodecHandlerE) error {
		switch {
		case src.Type() == reflect.TypeFor[*P]() && dst.Type() == reflect.TypeFor[*D]():
			if src.IsNil() {
				dst.SetZero()
				return nil
			}
			d, err := c.Domain(path, *src.Interface().(*P))
			if err != nil {
				return err
			}
			dst.Set(reflect.ValueOf(&d))
			return nil
		case src.Type() == reflect.TypeFor[*D]() && dst.Type() == reflect.TypeFor[*P]():
			if src.IsNil() {
				dst.SetZero()
				return nil
			}
			d := *src.Interface().(*D)
			p, err := c.ProtoE(d)
			if err != nil {
				return rootProtoError(path, err)
			}
			dst.Set(reflect.ValueOf(&p))
			return nil
		case src.Type() == reflect.TypeFor[P]() && dst.Type() == reflect.TypeFor[D]():
			d, err := c.Domain(path, src.Interface().(P))
			if err != nil {
				return err
			}
			dst.Set(reflect.ValueOf(&d).Elem())
			return nil
		case src.Type() == reflect.TypeFor[D]() && dst.Type() == reflect.TypeFor[P]():
			p, err := c.ProtoE(src.Interface().(D))
			if err != nil {
				return rootProtoError(path, err)
			}
			dst.Set(reflect.ValueOf(p))
			return nil
		default:
			return next(path, src, dst)
		}
	}
}

// rootProtoError preserves ProtoE's reason and cause at the adapter's source path.
func rootProtoError(path string, err error) error {
	var fieldErr *coreerrs.FieldError
	if errors.As(err, &fieldErr) {
		return &coreerrs.FieldError{Field: path, Reason: fieldErr.Reason, Err: fieldErr.Err}
	}
	return err
}
