// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package durpb

import (
	"reflect"
	"time"
	"unsafe"

	"google.golang.org/protobuf/types/known/durationpb"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
	reflectutils "github.com/altessa-s/go-atlas/domain/converter/internal/reflect"
)

var (
	durationType   = reflect.TypeFor[durationpb.Duration]()
	goDurationType = reflect.TypeFor[time.Duration]()

	// Pre-computed field offsets for zero-alloc Duration read/write via unsafe.
	durSecondsOffset = func() uintptr {
		f, ok := durationType.FieldByName("Seconds")
		if !ok {
			panic("durpb: durationpb.Duration has no Seconds field")
		}
		return f.Offset
	}()
	durNanosOffset = func() uintptr {
		f, ok := durationType.FieldByName("Nanos")
		if !ok {
			panic("durpb: durationpb.Duration has no Nanos field")
		}
		return f.Offset
	}()
)

// options holds configuration options for Duration conversion behavior.
type options struct {
	ignoreZero bool
}

// Option defines a functional option for configuring Duration conversion behavior.
type Option func(o *options)

// WithIgnoreZero returns an option that skips conversion when the source value is zero.
// For Duration: nil or {seconds:0, nanos:0}. For time.Duration: 0.
func WithIgnoreZero() Option {
	return func(o *options) { o.ignoreZero = true }
}

// New creates a Codec for bidirectional conversion between durationpb.Duration and time.Duration.
//
// Supported conversions:
//   - durationpb.Duration <-> time.Duration
//   - Pointer variants (*durationpb.Duration <-> *time.Duration, etc.)
//
// time.Duration <-> int64 is intentionally not handled here: time.Duration has an
// int64 kind, so the converter's built-in convertible path already maps it.
//
// Example:
//
//	codec := durpb.New()
//	codec := durpb.New(durpb.WithIgnoreZero())
func New(opts ...Option) convcodec.Codec {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	return func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		if tryConvert(src, dst, o) {
			return
		}

		next(fieldName, src, dst)
	}
}

// tryConvert attempts to convert between durationpb.Duration and time.Duration.
// Returns true if conversion was handled (including skipped due to options), false otherwise.
func tryConvert(src, dst reflect.Value, opts *options) bool {
	srcType := reflectutils.IndirectType(src.Type())
	dstType := reflectutils.IndirectType(dst.Type())

	srcValue := reflect.Indirect(src)

	switch {
	case srcType == durationType && dstType == goDurationType:
		return convertDurationToGo(srcValue, dst, dstType, opts)

	case srcType == goDurationType && dstType == durationType:
		return convertGoToDuration(srcValue, dst, dstType, opts)

	default:
		return false
	}
}

// convertDurationToGo handles durationpb.Duration -> time.Duration conversion.
func convertDurationToGo(srcValue reflect.Value, dst reflect.Value, dstType reflect.Type, opts *options) bool {
	if !srcValue.IsValid() {
		return true
	}

	sec, nanos, ok := readDuration(srcValue)
	if !ok {
		return false
	}

	if opts.ignoreZero && sec == 0 && nanos == 0 {
		return true
	}

	d := time.Duration(sec)*time.Second + time.Duration(nanos)*time.Nanosecond

	reflectutils.MakeDst(&dst, dstType)
	dst.SetInt(int64(d))

	return true
}

// convertGoToDuration handles time.Duration -> durationpb.Duration conversion.
func convertGoToDuration(srcValue reflect.Value, dst reflect.Value, dstType reflect.Type, opts *options) bool {
	if !srcValue.IsValid() {
		return true
	}

	d := time.Duration(srcValue.Int())

	if opts.ignoreZero && d == 0 {
		return true
	}

	// Split toward zero, matching durationpb.New: seconds carries the whole part,
	// nanos the signed remainder.
	sec := int64(d / time.Second)
	nanos := int32(d % time.Second)

	reflectutils.MakeDst(&dst, dstType)
	writeDuration(dst, sec, nanos)

	return true
}

// readDuration extracts Seconds and Nanos from a Duration reflect.Value
// without allocating. Uses unsafe pointer arithmetic when the value is addressable,
// falls back to reflect field access otherwise.
//
// #nosec G103 -- intentional unsafe for zero-alloc Duration read with verified safety
func readDuration(v reflect.Value) (seconds int64, nanos int32, ok bool) {
	if v.CanAddr() {
		base := unsafe.Pointer(v.UnsafeAddr())
		seconds = *(*int64)(unsafe.Add(base, durSecondsOffset))
		nanos = *(*int32)(unsafe.Add(base, durNanosOffset))

		return seconds, nanos, true
	}

	// Non-addressable fallback: use reflect field access. Reading by field avoids
	// copying the Duration value, which embeds a sync.Mutex via protoimpl.MessageState.
	secField := v.FieldByName("Seconds")
	nanField := v.FieldByName("Nanos")
	if !secField.IsValid() || !nanField.IsValid() {
		return 0, 0, false
	}

	return secField.Int(), int32(nanField.Int()), true
}

// writeDuration writes Seconds and Nanos directly into a Duration reflect.Value
// without allocating via durationpb.New() + reflect.ValueOf().
// Uses unsafe pointer arithmetic when the value is addressable,
// falls back to durationpb construction otherwise.
//
// #nosec G103 -- intentional unsafe for zero-alloc Duration write with verified safety
func writeDuration(dst reflect.Value, seconds int64, nanos int32) {
	if dst.CanAddr() {
		base := unsafe.Pointer(dst.UnsafeAddr())
		*(*int64)(unsafe.Add(base, durSecondsOffset)) = seconds
		*(*int32)(unsafe.Add(base, durNanosOffset)) = nanos

		return
	}

	d := &durationpb.Duration{Seconds: seconds, Nanos: nanos}
	dst.Set(reflect.ValueOf(d).Elem())
}
