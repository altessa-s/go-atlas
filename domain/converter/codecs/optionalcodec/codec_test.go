// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optionalcodec_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/optional"
	"github.com/altessa-s/go-atlas/domain/converter"
	"github.com/altessa-s/go-atlas/domain/converter/codecs/optionalcodec"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
)

// failingHandler is the terminal CodecHandler used by direct codec tests
// where the codec is expected to handle the conversion itself.
func failingHandler(t *testing.T) convcodec.CodecHandler {
	return func(fieldName string, src, dst reflect.Value) {
		t.Helper()
		t.Fatalf("codec delegated to handler for %q (src=%v dst=%v): Codec should have handled the pair",
			fieldName, src.Type(), dst.Type())
	}
}

func TestCodec_OptionalToPointer_Some(t *testing.T) {
	t.Parallel()

	src := reflect.ValueOf(optional.Some("hi"))

	var dstPtr *string
	dst := reflect.ValueOf(&dstPtr).Elem()

	optionalcodec.Codec("f", src, dst, failingHandler(t))
	require.NotNil(t, dstPtr)
	require.Equal(t, "hi", *dstPtr)
}

func TestCodec_OptionalToPointer_None(t *testing.T) {
	t.Parallel()

	src := reflect.ValueOf(optional.None[string]())

	dstPtr := testhelpers.StringPtr("seed")
	dst := reflect.ValueOf(&dstPtr).Elem()

	optionalcodec.Codec("f", src, dst, failingHandler(t))
	require.Nil(t, dstPtr)
}

func TestCodec_PointerToOptional_Nil(t *testing.T) {
	t.Parallel()

	var srcPtr *string
	src := reflect.ValueOf(&srcPtr).Elem()

	var dst optional.Optional[string]
	dstV := reflect.ValueOf(&dst).Elem()

	optionalcodec.Codec("f", src, dstV, failingHandler(t))
	require.True(t, dst.IsNone())
}

func TestCodec_PointerToOptional_NonNil(t *testing.T) {
	t.Parallel()

	srcPtr := testhelpers.StringPtr("hi")
	src := reflect.ValueOf(&srcPtr).Elem()

	var dst optional.Optional[string]
	dstV := reflect.ValueOf(&dst).Elem()

	optionalcodec.Codec("f", src, dstV, failingHandler(t))
	require.True(t, dst.IsSome())
	require.Equal(t, "hi", dst.Value())
}

func TestCodec_OptionalToOptional_SameType(t *testing.T) {
	t.Parallel()

	src := reflect.ValueOf(optional.Some(42))

	var dst optional.Optional[int]
	dstV := reflect.ValueOf(&dst).Elem()

	optionalcodec.Codec("f", src, dstV, failingHandler(t))
	require.True(t, dst.IsSome())
	require.Equal(t, 42, dst.Value())
}

func TestCodec_OptionalToValue_Some(t *testing.T) {
	t.Parallel()

	src := reflect.ValueOf(optional.Some(7))

	var dst int
	dstV := reflect.ValueOf(&dst).Elem()

	optionalcodec.Codec("f", src, dstV, failingHandler(t))
	require.Equal(t, 7, dst)
}

func TestCodec_OptionalToValue_None(t *testing.T) {
	t.Parallel()

	src := reflect.ValueOf(optional.None[int]())

	dst := 99
	dstV := reflect.ValueOf(&dst).Elem()

	optionalcodec.Codec("f", src, dstV, failingHandler(t))
	require.Equal(t, 0, dst, "None collapses to zero T")
}

func TestCodec_ValueToOptional(t *testing.T) {
	t.Parallel()

	src := reflect.ValueOf("hi")

	var dst optional.Optional[string]
	dstV := reflect.ValueOf(&dst).Elem()

	optionalcodec.Codec("f", src, dstV, failingHandler(t))
	require.True(t, dst.IsSome())
	require.Equal(t, "hi", dst.Value())
}

func TestCodec_DelegatesOnUnrelatedTypes(t *testing.T) {
	t.Parallel()

	src := reflect.ValueOf(int64(7))

	var dst int32
	dstV := reflect.ValueOf(&dst).Elem()

	var called bool
	terminal := func(fieldName string, src, dst reflect.Value) {
		called = true
	}
	optionalcodec.Codec("f", src, dstV, terminal)
	require.True(t, called, "Codec must delegate when neither side is Optional")
}

func TestCodec_DelegatesOnMismatchedOptionalInner(t *testing.T) {
	t.Parallel()

	src := reflect.ValueOf(optional.Some("hi"))

	var dstPtr *int
	dst := reflect.ValueOf(&dstPtr).Elem()

	var called bool
	terminal := func(fieldName string, src, dst reflect.Value) {
		called = true
	}
	optionalcodec.Codec("f", src, dst, terminal)
	require.True(t, called)
}

// Integration tests below verify the codec works through the real
// converter pipeline.

func TestCodec_IntegrationViaConverter_PointerToOptional(t *testing.T) {
	t.Parallel()

	type model struct {
		DeletedAt *time.Time
		Note      *string
	}
	type entity struct {
		DeletedAt optional.Optional[time.Time]
		Note      optional.Optional[string]
	}

	conv := converter.New[model, *entity](
		converter.WithHandleEmbeddedStructs(true),
		converter.WithCodecs(optionalcodec.Codec),
	)

	when := time.Date(2026, 6, 3, 14, 0, 0, 0, time.UTC)
	note := "hello"
	src := model{DeletedAt: &when, Note: &note}

	var dst entity
	conv.Convert(src, &dst)

	gotTime, ok := dst.DeletedAt.Get()
	require.True(t, ok)
	require.Equal(t, when, gotTime)

	gotNote, ok := dst.Note.Get()
	require.True(t, ok)
	require.Equal(t, "hello", gotNote)
}

func TestCodec_IntegrationViaConverter_NilPointersBecomeNone(t *testing.T) {
	t.Parallel()

	type model struct {
		DeletedAt *time.Time
	}
	type entity struct {
		DeletedAt optional.Optional[time.Time]
	}

	conv := converter.New[model, *entity](
		converter.WithHandleEmbeddedStructs(true),
		converter.WithCodecs(optionalcodec.Codec),
	)

	var dst entity
	conv.Convert(model{}, &dst)
	require.True(t, dst.DeletedAt.IsNone())
}

func TestCodec_IntegrationViaConverter_OptionalToPointer(t *testing.T) {
	t.Parallel()

	type entity struct {
		DeletedAt optional.Optional[time.Time]
	}
	type model struct {
		DeletedAt *time.Time
	}

	conv := converter.New[entity, *model](
		converter.WithHandleEmbeddedStructs(true),
		converter.WithCodecs(optionalcodec.Codec),
	)

	when := time.Date(2026, 6, 3, 14, 0, 0, 0, time.UTC)
	src := entity{DeletedAt: optional.Some(when)}

	var dst model
	conv.Convert(src, &dst)
	require.NotNil(t, dst.DeletedAt)
	require.Equal(t, when, *dst.DeletedAt)
}

func TestCodec_IntegrationViaConverter_NoneToNilPointer(t *testing.T) {
	t.Parallel()

	type entity struct {
		DeletedAt optional.Optional[time.Time]
	}
	type model struct {
		DeletedAt *time.Time
	}

	conv := converter.New[entity, *model](
		converter.WithHandleEmbeddedStructs(true),
		converter.WithCodecs(optionalcodec.Codec),
	)

	src := entity{DeletedAt: optional.None[time.Time]()}

	dst := model{DeletedAt: testhelpers.TimePtr(time.Now())} // pre-set, must be cleared
	conv.Convert(src, &dst)
	require.Nil(t, dst.DeletedAt)
}
