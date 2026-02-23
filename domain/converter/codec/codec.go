// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convcodec

import (
	"reflect"
	"sync"
)

// Codec is a middleware-style conversion function. It receives the current
// field name, source and destination [reflect.Value] objects, and a next
// [CodecHandler] that represents the rest of the chain. A Codec should either
// handle the conversion (set dst) and return, or delegate to next.
//
// Example:
//
//	codec := func(fieldName string, src, dst reflect.Value, next CodecHandler) {
//	    if shouldHandle(src, dst) {
//	        dst.Set(transformValue(src))
//	    } else {
//	        next(fieldName, src, dst)
//	    }
//	}
type Codec func(fieldName string, src, dst reflect.Value, next CodecHandler)

// CodecHandler is the terminal callback in a codec chain. It performs the
// default conversion logic when no [Codec] in the chain handles the field.
type CodecHandler func(fieldName string, src, dst reflect.Value)

// Set represents an ordered collection of codecs that form a conversion pipeline.
// Codecs execute in registration order; each codec can handle the conversion or
// delegate to the next via the CodecHandler callback. Not safe for concurrent
// modification after construction; concurrent [Set.Run] calls are safe.
type Set struct {
	customCodecs []Codec
}

// NewCodecsSet creates and returns a new codec Set initialized with the provided codecs.
// The codecs will be executed in the order they are provided when the Set is used for conversions.
//
// Example:
//
//	codecSet := NewCodecsSet(customCodec1, customCodec2, customCodec3)
func NewCodecsSet(c ...Codec) *Set {
	return (&Set{}).Add(c...)
}

// Add appends one or more codecs to the existing codec Set.
// The newly added codecs will be executed after the existing ones in the conversion chain.
// Returns the Set itself to enable method chaining.
//
// Example:
//
//	codecSet.Add(codec1).Add(codec2, codec3)
func (c *Set) Add(codecs ...Codec) *Set {
	c.customCodecs = append(c.customCodecs, codecs...)
	return c
}

// Run executes the codec chain for the given field and values.
// It processes all codecs in order, with each having the opportunity to handle the conversion.
// The finishHandler is called at the end if no codec handles the conversion.
//
// Example:
//
//	codecSet.Run("name", srcValue, dstValue, defaultHandler)
func (c *Set) Run(fieldName string, src, dst reflect.Value, finishHandler CodecHandler) {
	switch len(c.customCodecs) {
	case 0:
		finishHandler(fieldName, src, dst)
	case 1:
		c.customCodecs[0](fieldName, src, dst, finishHandler)
	default:
		ctx, ok := codecCtxPool.Get().(*codecContext)
		if !ok {
			ctx = &codecContext{}
			ctx.nextFn = ctx.next
		}
		ctx.codecs = c.customCodecs
		ctx.final = finishHandler
		ctx.index = 1

		c.customCodecs[0](fieldName, src, dst, ctx.nextFn)

		ctx.codecs = nil
		ctx.final = nil
		codecCtxPool.Put(ctx)
	}
}

// HasCodecs reports whether the Set contains any custom codecs.
// Returns true if one or more codecs have been added to the Set, false otherwise.
// This is useful for optimization - if no codecs are present, conversion can skip codec processing entirely.
func (c *Set) HasCodecs() bool {
	return len(c.customCodecs) > 0
}

// codecContext is a pooled execution context for multi-codec chains.
// Each context pre-binds its next method as a CodecHandler to avoid
// allocating a new closure on every Run call.
type codecContext struct {
	codecs []Codec
	final  CodecHandler
	index  int
	nextFn CodecHandler // pre-bound method value, allocated once per pool entry
}

// codecCtxPool reuses codecContext instances across Run calls.
var codecCtxPool = sync.Pool{
	New: func() any {
		ctx := &codecContext{}
		ctx.nextFn = ctx.next
		return ctx
	},
}

// next advances the chain to the next codec, or calls the final handler
// when all codecs have been visited.
func (ctx *codecContext) next(fieldName string, src, dst reflect.Value) {
	i := ctx.index
	if i >= len(ctx.codecs) {
		ctx.final(fieldName, src, dst)
		return
	}
	ctx.index = i + 1
	ctx.codecs[i](fieldName, src, dst, ctx.nextFn)
}
