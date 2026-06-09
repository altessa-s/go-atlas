// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optionalcodec_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/types/optional"
	"github.com/altessa-s/go-atlas/domain/converter"
	"github.com/altessa-s/go-atlas/domain/converter/codec/durpb"
	"github.com/altessa-s/go-atlas/domain/converter/codec/tspb"
	"github.com/altessa-s/go-atlas/domain/converter/codecs/optionalcodec"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

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

// Compose benchmarks exercise the full converter chain (optionalcodec + tspb +
// durpb) end-to-end. They are deliberately not invoking the codec directly via
// reflect — composition only fires through the chain machinery in
// domain/converter/codec.Set.Run, and that is the path we want to measure.

type benchComposeDomain struct {
	DeleteTime optional.Optional[time.Time]
	TTL        optional.Optional[time.Duration]
}

type benchComposeProto struct {
	DeleteTime *timestamppb.Timestamp
	TTL        *durationpb.Duration
}

var benchComposeToProtoOpts = []converter.Option{
	converter.WithCodecs(
		optionalcodec.Codec,
		tspb.New(tspb.WithIgnoreZero()),
		durpb.New(durpb.WithIgnoreZero()),
	),
	converter.WithIgnoreZeroValues(),
}

var benchComposeFromProtoOpts = []converter.Option{
	converter.WithCodecs(
		optionalcodec.Codec,
		tspb.New(tspb.WithIgnoreZero()),
		durpb.New(durpb.WithIgnoreZero()),
	),
}

func BenchmarkCompose_OptionalToProto_Some(b *testing.B) {
	src := &benchComposeDomain{
		DeleteTime: optional.Some(time.Unix(1700000000, 0).UTC()),
		TTL:        optional.Some(90 * time.Minute),
	}

	for b.Loop() {
		var pb benchComposeProto
		converter.Convert(src, &pb, benchComposeToProtoOpts...)
	}
}

func BenchmarkCompose_OptionalToProto_None(b *testing.B) {
	src := &benchComposeDomain{
		DeleteTime: optional.None[time.Time](),
		TTL:        optional.None[time.Duration](),
	}

	for b.Loop() {
		var pb benchComposeProto
		converter.Convert(src, &pb, benchComposeToProtoOpts...)
	}
}

func BenchmarkCompose_ProtoToOptional_Present(b *testing.B) {
	src := &benchComposeProto{
		DeleteTime: timestamppb.New(time.Unix(1700000000, 0).UTC()),
		TTL:        durationpb.New(90 * time.Minute),
	}

	for b.Loop() {
		var dom benchComposeDomain
		converter.Convert(src, &dom, benchComposeFromProtoOpts...)
	}
}

func BenchmarkCompose_ProtoToOptional_Nil(b *testing.B) {
	src := &benchComposeProto{DeleteTime: nil, TTL: nil}

	for b.Loop() {
		var dom benchComposeDomain
		converter.Convert(src, &dom, benchComposeFromProtoOpts...)
	}
}
