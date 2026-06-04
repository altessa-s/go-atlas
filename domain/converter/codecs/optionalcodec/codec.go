// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
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
// registered with converter.WithCodecs(optionalcodec.Codec).
//
// Supported conversions, all in both directions:
//
//   - Optional[T] ↔ Optional[T]: direct assignment.
//   - Optional[T] ↔ *T:           None ↔ nil pointer; Some(v) ↔ &v.
//   - Optional[T] ↔ T:            None ↔ zero T; Some(v) ↔ v.
//
// When the source / destination shapes do not match one of the above
// patterns the codec delegates to the chain via next, leaving other
// codecs and the built-in field-by-field copy untouched.
//
// The codec is safe for concurrent use and allocation-light: it builds
// at most one reflect.Value per call (and only when an Optional has to
// be constructed or extracted), reusing the destination slot in every
// other case.
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

	default:
		next(fieldName, src, dst)
	}
}
