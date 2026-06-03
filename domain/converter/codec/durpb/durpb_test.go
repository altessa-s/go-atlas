// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package durpb_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/converter"
	"github.com/altessa-s/go-atlas/domain/converter/codec/durpb"

	"google.golang.org/protobuf/types/known/durationpb"
)

func TestNew_DurationToGo(t *testing.T) {
	t.Parallel()

	d := 90*time.Minute + 500*time.Millisecond
	pb := durationpb.New(d)
	codec := durpb.New()

	src := reflect.ValueOf(pb).Elem()
	var dst time.Duration
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Ttl", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, d, dst)
}

func TestNew_GoToDuration(t *testing.T) {
	t.Parallel()

	d := 90*time.Minute + 500*time.Millisecond
	codec := durpb.New()

	src := reflect.ValueOf(d)
	var dst durationpb.Duration
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Ttl", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, d, dst.AsDuration())
}

func TestNew_NegativeDuration(t *testing.T) {
	t.Parallel()

	d := -(90*time.Minute + 500*time.Millisecond)
	codec := durpb.New()

	// go -> pb
	src := reflect.ValueOf(d)
	var pb durationpb.Duration
	codec("Ttl", src, reflect.ValueOf(&pb).Elem(), func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})
	assert.Equal(t, d, pb.AsDuration())

	// pb -> go (round trip)
	var back time.Duration
	codec("Ttl", reflect.ValueOf(&pb).Elem(), reflect.ValueOf(&back).Elem(), func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})
	assert.Equal(t, d, back)
}

func TestNew_Pointer_DurationToGo(t *testing.T) {
	t.Parallel()

	d := 3 * time.Second
	pb := durationpb.New(d)
	codec := durpb.New()

	src := reflect.ValueOf(pb) // *durationpb.Duration
	var dst *time.Duration
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Ttl", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	require.NotNil(t, dst)
	assert.Equal(t, d, *dst)
}

func TestNew_Pointer_GoToDuration(t *testing.T) {
	t.Parallel()

	d := 3 * time.Second
	codec := durpb.New()

	src := reflect.ValueOf(&d) // *time.Duration
	var dst *durationpb.Duration
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Ttl", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	require.NotNil(t, dst)
	assert.Equal(t, d, dst.AsDuration())
}

func TestNew_IgnoreZero_GoToDuration(t *testing.T) {
	t.Parallel()

	codec := durpb.New(durpb.WithIgnoreZero())

	var dst durationpb.Duration
	dstVal := reflect.ValueOf(&dst).Elem()

	// Zero source is skipped (handled, next not called), dst stays zero.
	codec("Ttl", reflect.ValueOf(time.Duration(0)), dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, time.Duration(0), dst.AsDuration())
}

func TestNew_IgnoreZero_DurationToGo(t *testing.T) {
	t.Parallel()

	codec := durpb.New(durpb.WithIgnoreZero())

	var pb durationpb.Duration // {0,0}
	var dst time.Duration = 42 // sentinel: must stay untouched
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Ttl", reflect.ValueOf(&pb).Elem(), dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, time.Duration(42), dst)
}

func TestNew_Unrelated_DelegatesToNext(t *testing.T) {
	t.Parallel()

	codec := durpb.New()
	called := false
	next := func(_ string, _, _ reflect.Value) { called = true }

	src := reflect.ValueOf("not a duration")
	var dst string
	codec("Field", src, reflect.ValueOf(&dst).Elem(), next)

	assert.True(t, called, "next must be called for non-duration types")
}

func TestNew_NilPointer_DurationToGo(t *testing.T) {
	t.Parallel()

	codec := durpb.New()

	var src *durationpb.Duration
	srcVal := reflect.ValueOf(src)
	var dst *time.Duration
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Ttl", srcVal, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Nil(t, dst, "dst should remain nil when src is nil pointer")
}

func TestNew_NilPointer_GoToDuration(t *testing.T) {
	t.Parallel()

	codec := durpb.New()

	var src *time.Duration
	srcVal := reflect.ValueOf(src)
	var dst *durationpb.Duration
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Ttl", srcVal, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Nil(t, dst, "dst should remain nil when src is nil pointer")
}

// --- end-to-end through converter.Convert ---

type durDomain struct {
	Name   string
	TTL    time.Duration
	Grace  *time.Duration
	Splits []time.Duration
	Meta   map[string]time.Duration
}

type durProto struct {
	Name   string
	TTL    *durationpb.Duration
	Grace  *durationpb.Duration
	Splits []*durationpb.Duration
	Meta   map[string]*durationpb.Duration
}

func TestConverter_DomainToProto(t *testing.T) {
	t.Parallel()

	grace := 30 * time.Second
	in := durDomain{Name: "n", TTL: 5 * time.Minute, Grace: &grace}

	out := converter.Convert(in, &durProto{},
		converter.WithIgnoreZeroValues(),
		converter.WithCodecs(durpb.New(durpb.WithIgnoreZero())),
	)

	assert.Equal(t, "n", out.Name)
	require.NotNil(t, out.TTL)
	assert.Equal(t, 5*time.Minute, out.TTL.AsDuration())
	require.NotNil(t, out.Grace)
	assert.Equal(t, grace, out.Grace.AsDuration())
}

func TestConverter_DomainToProto_ZeroStaysNil(t *testing.T) {
	t.Parallel()

	in := durDomain{Name: "n"} // TTL zero, Grace nil

	out := converter.Convert(in, &durProto{},
		converter.WithIgnoreZeroValues(),
		converter.WithCodecs(durpb.New(durpb.WithIgnoreZero())),
	)

	assert.Nil(t, out.TTL, "zero duration must stay nil in proto")
	assert.Nil(t, out.Grace, "nil pointer must stay nil in proto")
}

func TestConverter_ProtoToDomain(t *testing.T) {
	t.Parallel()

	grace := 30 * time.Second
	in := durProto{Name: "n", TTL: durationpb.New(5 * time.Minute), Grace: durationpb.New(grace)}

	out := converter.Convert(in, &durDomain{},
		converter.WithCodecs(durpb.New(durpb.WithIgnoreZero())),
	)

	assert.Equal(t, "n", out.Name)
	assert.Equal(t, 5*time.Minute, out.TTL)
	require.NotNil(t, out.Grace)
	assert.Equal(t, grace, *out.Grace)
}

// TestCodec_ThroughConverter exercises the durpb codec end-to-end through the
// converter (not by calling it directly), covering both directions and
// slice / map elements. This guards against the converter dispatching
// struct-kind fields field-by-field before consulting codecs, which would
// silently zero time.Duration <-> Duration.
func TestCodec_ThroughConverter(t *testing.T) {
	t.Parallel()

	d1 := 90*time.Minute + 500*time.Millisecond
	d2 := 250 * time.Millisecond

	t.Run("time.Duration to Duration", func(t *testing.T) {
		t.Parallel()

		src := durDomain{
			Name:   "e1",
			TTL:    d1,
			Splits: []time.Duration{d1, d2},
			Meta:   map[string]time.Duration{"k": d2},
		}
		var dst durProto
		converter.Convert(src, &dst, converter.WithCodecs(durpb.New()))

		assert.Equal(t, "e1", dst.Name)
		require.NotNil(t, dst.TTL)
		assert.Equal(t, d1, dst.TTL.AsDuration())
		require.Len(t, dst.Splits, 2)
		assert.Equal(t, d1, dst.Splits[0].AsDuration())
		assert.Equal(t, d2, dst.Splits[1].AsDuration())
		require.Contains(t, dst.Meta, "k")
		assert.Equal(t, d2, dst.Meta["k"].AsDuration())
	})

	t.Run("Duration to time.Duration", func(t *testing.T) {
		t.Parallel()

		src := durProto{
			Name:   "e2",
			TTL:    durationpb.New(d1),
			Splits: []*durationpb.Duration{durationpb.New(d1), durationpb.New(d2)},
			Meta:   map[string]*durationpb.Duration{"k": durationpb.New(d2)},
		}
		var dst durDomain
		converter.Convert(src, &dst, converter.WithCodecs(durpb.New()))

		assert.Equal(t, "e2", dst.Name)
		assert.Equal(t, d1, dst.TTL)
		require.Len(t, dst.Splits, 2)
		assert.Equal(t, d1, dst.Splits[0])
		assert.Equal(t, d2, dst.Splits[1])
		require.Contains(t, dst.Meta, "k")
		assert.Equal(t, d2, dst.Meta["k"])
	})
}
