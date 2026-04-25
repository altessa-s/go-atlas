// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jsonpb

import (
	"encoding/json"
	"reflect"

	"google.golang.org/protobuf/types/known/structpb"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
	reflectutils "github.com/altessa-s/go-atlas/domain/converter/internal/reflect"
)

var (
	rawMessageType = reflect.TypeFor[json.RawMessage]()
	structpbType   = reflect.TypeFor[structpb.Struct]()
)

// options holds configuration options for JSON <-> structpb.Struct conversion behavior.
type options struct {
	ignoreNil bool
}

// Option defines a functional option for configuring JSON <-> structpb.Struct conversion behavior.
type Option func(o *options)

// WithIgnoreNil returns an option that skips conversion when the source pointer is nil.
// Without this option, a nil source still marks the conversion as handled (destination stays zero-value).
// With this option the behavior is identical, but the intent is explicit.
func WithIgnoreNil() Option {
	return func(o *options) { o.ignoreNil = true }
}

// New creates a Codec for bidirectional conversion between json.RawMessage and structpb.Struct.
//
// Supported conversions (pointer fields only):
//   - *json.RawMessage -> *structpb.Struct: unmarshals JSON into a map then builds a Struct
//   - *structpb.Struct -> *json.RawMessage: marshals the Struct to JSON bytes
//
// Non-pointer variants (json.RawMessage <-> structpb.Struct) are also supported.
//
// Example:
//
//	codec := jsonpb.New()
//	codec := jsonpb.New(jsonpb.WithIgnoreNil())
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

// tryConvert attempts to convert between json.RawMessage and structpb.Struct.
// Returns true if conversion was handled, false otherwise.
func tryConvert(src, dst reflect.Value, opts *options) bool {
	srcType := reflectutils.IndirectType(src.Type())
	dstType := reflectutils.IndirectType(dst.Type())

	srcValue := reflect.Indirect(src)

	switch {
	case srcType == rawMessageType && dstType == structpbType:
		return convertRawMessageToStruct(src, srcValue, dst, dstType, opts)

	case srcType == structpbType && dstType == rawMessageType:
		return convertStructToRawMessage(src, srcValue, dst, dstType, opts)

	default:
		return false
	}
}

// convertRawMessageToStruct handles json.RawMessage -> structpb.Struct conversion.
func convertRawMessageToStruct(src, srcValue, dst reflect.Value, dstType reflect.Type, opts *options) bool {
	// Handle nil pointer or nil/empty slice
	if !srcValue.IsValid() || srcValue.IsNil() || srcValue.Len() == 0 {
		return true
	}

	raw, ok := srcValue.Interface().(json.RawMessage)
	if !ok {
		return false
	}

	if opts.ignoreNil && src.Kind() == reflect.Pointer && src.IsNil() {
		return true
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}

	pb, err := structpb.NewStruct(m)
	if err != nil {
		return false
	}

	reflectutils.MakeDst(&dst, dstType)
	dst.Set(reflect.ValueOf(pb).Elem())

	return true
}

// convertStructToRawMessage handles structpb.Struct -> json.RawMessage conversion.
func convertStructToRawMessage(src, srcValue, dst reflect.Value, dstType reflect.Type, opts *options) bool {
	// Handle nil pointer source
	if !srcValue.IsValid() {
		return true
	}

	if opts.ignoreNil && src.Kind() == reflect.Pointer && src.IsNil() {
		return true
	}

	var pb *structpb.Struct

	if srcValue.CanAddr() {
		var ok bool

		pb, ok = srcValue.Addr().Interface().(*structpb.Struct)
		if !ok {
			return false
		}
	} else {
		// Value is not addressable (e.g. passed directly via reflect.ValueOf).
		// Create an addressable copy.
		tmp := reflect.New(structpbType)
		tmp.Elem().Set(srcValue)

		var ok bool

		pb, ok = tmp.Interface().(*structpb.Struct)
		if !ok {
			return false
		}
	}

	data, err := json.Marshal(pb.AsMap())
	if err != nil {
		return false
	}

	raw := json.RawMessage(data)

	reflectutils.MakeDst(&dst, dstType)
	dst.Set(reflect.ValueOf(raw))

	return true
}
