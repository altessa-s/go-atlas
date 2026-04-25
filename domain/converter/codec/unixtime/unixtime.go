// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package unixtime

import (
	"reflect"
	"time"
	"unsafe"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
	reflectutils "github.com/altessa-s/go-atlas/domain/converter/internal/reflect"
)

var (
	timeType  = reflect.TypeFor[time.Time]()
	int64Type = reflect.TypeFor[int64]()
)

// options holds configuration options for Unix time conversion behavior.
type options struct {
	ignoreZero   bool
	milliseconds bool
}

// Option defines a functional option for configuring Unix time conversion behavior.
type Option func(o *options)

// WithIgnoreZero returns an option that skips conversion when the source value is zero.
// For time.Time sources, zero means time.Time{}. For int64 sources, zero means 0.
func WithIgnoreZero() Option {
	return func(o *options) { o.ignoreZero = true }
}

// WithMilliseconds returns an option that uses millisecond precision instead of seconds.
// time.Time -> int64 uses UnixMilli(), int64 -> time.Time uses time.UnixMilli().
func WithMilliseconds() Option {
	return func(o *options) { o.milliseconds = true }
}

// New creates a Codec for bidirectional conversion between time.Time and int64 Unix timestamps.
// By default, timestamps use second precision. Use WithMilliseconds for millisecond precision.
//
// Supported conversions:
//   - time.Time -> int64: calls t.Unix() (or t.UnixMilli())
//   - int64 -> time.Time: calls time.Unix(n, 0) (or time.UnixMilli(n))
//   - Pointer variants (*time.Time <-> *int64) are also supported
//
// Example:
//
//	codec := unixtime.New()
//	codec := unixtime.New(unixtime.WithIgnoreZero(), unixtime.WithMilliseconds())
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

// tryConvert attempts to convert between time.Time and int64.
// Returns true if conversion was handled (including skipped due to options), false otherwise.
func tryConvert(src, dst reflect.Value, opts *options) bool {
	srcType := reflectutils.IndirectType(src.Type())
	dstType := reflectutils.IndirectType(dst.Type())

	srcValue := reflect.Indirect(src)

	switch {
	case srcType == timeType && dstType == int64Type:
		// Handle nil pointer source
		if !srcValue.IsValid() {
			return true
		}

		t, ok := readTime(srcValue)
		if !ok {
			return false
		}

		if opts.ignoreZero && t.IsZero() {
			return true
		}

		var unix int64
		if opts.milliseconds {
			unix = t.UnixMilli()
		} else {
			unix = t.Unix()
		}

		reflectutils.MakeDst(&dst, dstType)
		dst.SetInt(unix)

		return true

	case srcType == int64Type && dstType == timeType:
		// Handle nil pointer source
		if !srcValue.IsValid() {
			return true
		}

		v := srcValue.Int()

		if opts.ignoreZero && v == 0 {
			return true
		}

		var t time.Time
		if opts.milliseconds {
			t = time.UnixMilli(v)
		} else {
			t = time.Unix(v, 0)
		}

		reflectutils.MakeDst(&dst, dstType)
		writeTime(dst, t)

		return true

	default:
		return false
	}
}

// readTime extracts time.Time from a reflect.Value without heap-allocating
// via Interface() boxing. Falls back to Interface() for non-addressable values.
//
// #nosec G103 -- intentional unsafe for zero-alloc time.Time read with verified safety
func readTime(v reflect.Value) (time.Time, bool) {
	if v.CanAddr() {
		return *(*time.Time)(unsafe.Pointer(v.UnsafeAddr())), true
	}

	t, ok := v.Interface().(time.Time)
	return t, ok
}

// writeTime sets a time.Time into a reflect.Value without heap-allocating
// via reflect.ValueOf(). Falls back to reflect.ValueOf for non-addressable values.
//
// #nosec G103 -- intentional unsafe for zero-alloc time.Time write with verified safety
func writeTime(dst reflect.Value, t time.Time) {
	if dst.CanAddr() {
		*(*time.Time)(unsafe.Pointer(dst.UnsafeAddr())) = t
		return
	}

	dst.Set(reflect.ValueOf(t))
}
