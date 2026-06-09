// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optionalcodec

import (
	"reflect"

	"github.com/altessa-s/go-atlas/core/types/optional"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
)

// Codec bridges optional.Optional[T] fields and matching pointer / value
// counterparts during struct-to-struct conversion. It is intended to be
// registered with converter.WithCodecs(optionalcodec.Codec), and — so that
// composition (below) works — placed BEFORE any type-specific codec it should
// compose with (e.g. tspb, durpb).
//
// Direct shapes, all in both directions:
//
//   - Optional[T] ↔ Optional[T]: direct assignment.
//   - Optional[T] ↔ *T:           None ↔ nil pointer; Some(v) ↔ &v.
//   - Optional[T] ↔ T:            None ↔ zero T; Some(v) ↔ v.
//
// Composition: when one side is Optional[T] and the other is some type W that
// the codec itself does not bridge (e.g. *timestamppb.Timestamp,
// *durationpb.Duration), the codec unwraps / wraps the Optional and delegates
// the inner T ↔ W conversion to the rest of the chain. This lets
// Optional[time.Time] ↔ *timestamppb.Timestamp work via tspb,
// Optional[time.Duration] ↔ *durationpb.Duration via durpb, and so on, as long
// as a downstream codec handles the inner type. For the composition to fire,
// this codec must run before that downstream codec; if no downstream codec
// claims the inner T ↔ W pair the value is handed to the converter's terminal
// field-by-field copy, which will either no-op or fail depending on type
// compatibility — the caller is responsible for registering the right bridge.
//
// Optional[A] ↔ Optional[B] with different inner types is not handled: the pair
// is delegated to the chain unchanged. Register a custom codec for that bridge
// if you need it.
//
// On the W → Optional[T] side a converted inner that is the struct-zero value
// of T — as reported by reflect.Value.IsZero, i.e. an all-zero struct, not a
// type's own IsZero method — is treated as absent (None), matching the
// None ↔ nil/zero presence convention used by the direct shapes. The
// distinction matters for time.Time: a *timestamppb.Timestamp that tspb
// materializes into a located time.Time is struct-non-zero and stays Some;
// only true absence (a nil pointer, or a value tspb's WithIgnoreZero skips so
// the inner stays pristine) maps to None. For time.Duration the same rule
// means a non-nil *durationpb.Duration whose value is zero collapses to None —
// an observable consequence of the convention, not a special case. Callers
// that need to distinguish "explicit zero" from "absent" for such inner types
// should use a direct Optional[T] ↔ *T mapping instead of composing through a
// downstream codec that strips the wrapper.
//
// When neither a direct shape nor composition applies the codec delegates to
// the chain via next, leaving other codecs and the built-in field-by-field
// copy untouched.
//
// The codec is safe for concurrent use and allocation-light: a direct shape
// builds at most one reflect.Value (and only when an Optional has to be
// constructed or extracted); the composition path builds at most two (the
// materialized inner value plus the wrapping Optional). The destination slot is
// reused in every other case.
func Codec(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
	srcType := src.Type()
	dstType := dst.Type()
	srcIsOpt := optional.IsOptionalType(srcType)
	dstIsOpt := optional.IsOptionalType(dstType)

	if !srcIsOpt && !dstIsOpt {
		next(fieldName, src, dst)
		return
	}

	switch {
	case srcIsOpt && dstIsOpt && srcType == dstType:
		dst.Set(src)

	case srcIsOpt && dstType.Kind() == reflect.Pointer &&
		dstType.Elem() == optional.InnerType(srcType):
		value, present := optional.GetReflect(src)
		if !present {
			dst.SetZero()
			return
		}
		p := reflect.New(dstType.Elem())
		p.Elem().Set(value)
		dst.Set(p)

	case dstIsOpt && srcType.Kind() == reflect.Pointer &&
		srcType.Elem() == optional.InnerType(dstType):
		if src.IsNil() {
			dst.Set(optional.NoneReflect(dstType))
			return
		}
		dst.Set(optional.SomeReflect(dstType, src.Elem()))

	case srcIsOpt && dstType == optional.InnerType(srcType):
		value, _ := optional.GetReflect(src)
		dst.Set(value)

	case dstIsOpt && srcType == optional.InnerType(dstType):
		dst.Set(optional.SomeReflect(dstType, src))

	case srcIsOpt && !dstIsOpt:
		// Optional[T] -> W (W bridged by a downstream codec, e.g. *Timestamp).
		// Unwrap and delegate the inner T -> W conversion to the chain.
		value, present := optional.GetReflect(src)
		if !present {
			dst.SetZero()
			return
		}
		next(fieldName, value, dst)

	case dstIsOpt && !srcIsOpt:
		// W -> Optional[T]: convert W -> inner T via the chain, then wrap.
		// A struct-zero inner (reflect.Value.IsZero — all-zero value, not a
		// type's own IsZero method) is treated as absent (None).
		inner := reflect.New(optional.InnerType(dstType)).Elem()
		next(fieldName, src, inner)
		if inner.IsZero() {
			dst.Set(optional.NoneReflect(dstType))
			return
		}
		dst.Set(optional.SomeReflect(dstType, inner))

	default:
		next(fieldName, src, dst)
	}
}
