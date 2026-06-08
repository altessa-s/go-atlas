// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optionalcodec_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/types/optional"
	"github.com/altessa-s/go-atlas/domain/converter/codecs/optionalcodec"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
)

var benchNext convcodec.CodecHandler = func(string, reflect.Value, reflect.Value) {}

func BenchmarkCodec_OptionalToPointer_Some(b *testing.B) {
	src := reflect.ValueOf(optional.Some(time.Now().UTC()))

	var dstPtr *time.Time
	dst := reflect.ValueOf(&dstPtr).Elem()

	for b.Loop() {
		optionalcodec.Codec("f", src, dst, benchNext)
	}
}

func BenchmarkCodec_OptionalToPointer_None(b *testing.B) {
	src := reflect.ValueOf(optional.None[time.Time]())

	var dstPtr *time.Time
	dst := reflect.ValueOf(&dstPtr).Elem()

	for b.Loop() {
		optionalcodec.Codec("f", src, dst, benchNext)
	}
}

func BenchmarkCodec_PointerToOptional_NonNil(b *testing.B) {
	now := time.Now().UTC()
	srcPtr := &now
	src := reflect.ValueOf(&srcPtr).Elem()

	var dst optional.Optional[time.Time]
	dstV := reflect.ValueOf(&dst).Elem()

	for b.Loop() {
		optionalcodec.Codec("f", src, dstV, benchNext)
	}
}

func BenchmarkCodec_PointerToOptional_Nil(b *testing.B) {
	var srcPtr *time.Time
	src := reflect.ValueOf(&srcPtr).Elem()

	var dst optional.Optional[time.Time]
	dstV := reflect.ValueOf(&dst).Elem()

	for b.Loop() {
		optionalcodec.Codec("f", src, dstV, benchNext)
	}
}

func BenchmarkCodec_DelegatesUnrelated(b *testing.B) {
	src := reflect.ValueOf(int64(7))

	var dst int32
	dstV := reflect.ValueOf(&dst).Elem()

	for b.Loop() {
		optionalcodec.Codec("f", src, dstV, benchNext)
	}
}
