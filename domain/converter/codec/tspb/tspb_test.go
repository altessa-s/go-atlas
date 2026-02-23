// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tspb

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNew_TimestampToTime(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	ts := timestamppb.New(now)
	codec := New()

	src := reflect.ValueOf(ts).Elem()
	var dst time.Time
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.True(t, now.Equal(dst))
}

func TestNew_TimeToTimestamp(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	codec := New()

	src := reflect.ValueOf(now)
	var dst timestamppb.Timestamp
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.True(t, now.Equal(dst.AsTime()))
}

func TestNew_TimestampToInt64(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	ts := timestamppb.New(now)
	codec := New()

	src := reflect.ValueOf(ts).Elem()
	var dst int64
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Equal(t, now.Unix(), dst)
}

func TestNew_Int64ToTimestamp(t *testing.T) {
	var unix int64 = 1750000000
	codec := New()

	src := reflect.ValueOf(unix)
	var dst timestamppb.Timestamp
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Equal(t, unix, dst.AsTime().Unix())
}

func TestNew_Pointer_TimestampToTime(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	ts := timestamppb.New(now)
	codec := New()

	src := reflect.ValueOf(ts)
	var dst *time.Time
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	require.NotNil(t, dst)
	assert.True(t, now.Equal(*dst))
}

func TestNew_Pointer_TimeToTimestamp(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	codec := New()

	src := reflect.ValueOf(&now)
	var dst *timestamppb.Timestamp
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	require.NotNil(t, dst)
	assert.True(t, now.Equal(dst.AsTime()))
}

func TestNew_Pointer_TimestampToInt64(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	ts := timestamppb.New(now)
	codec := New()

	src := reflect.ValueOf(ts)
	var dst *int64
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	require.NotNil(t, dst)
	assert.Equal(t, now.Unix(), *dst)
}

func TestNew_Pointer_Int64ToTimestamp(t *testing.T) {
	var unix int64 = 1750000000
	codec := New()

	src := reflect.ValueOf(&unix)
	var dst *timestamppb.Timestamp
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	require.NotNil(t, dst)
	assert.Equal(t, unix, dst.AsTime().Unix())
}

func TestNew_IgnoreZero_Timestamp(t *testing.T) {
	codec := New(WithIgnoreZero())

	// Zero timestamp: seconds=0, nanos=0 (Unix epoch)
	ts := &timestamppb.Timestamp{}
	src := reflect.ValueOf(ts).Elem()
	original := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	dst := original
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Equal(t, original, dst, "dst should remain unchanged when zero timestamp is ignored")
}

func TestNew_IgnoreZero_Time(t *testing.T) {
	codec := New(WithIgnoreZero())

	src := reflect.ValueOf(time.Time{})
	orig := timestamppb.New(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	dstVal := reflect.ValueOf(orig).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Equal(t, int64(1735689600), orig.GetSeconds(), "dst should remain unchanged when zero time is ignored")
}

func TestNew_IgnoreZero_Int64(t *testing.T) {
	codec := New(WithIgnoreZero())

	src := reflect.ValueOf(int64(0))
	orig := timestamppb.New(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	dstVal := reflect.ValueOf(orig).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Equal(t, int64(1735689600), orig.GetSeconds(), "dst should remain unchanged when zero int64 is ignored")
}

func TestNew_Milliseconds(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	ts := timestamppb.New(now)
	codec := New(WithMilliseconds())

	// Timestamp -> int64 (milliseconds)
	src := reflect.ValueOf(ts).Elem()
	var dst int64
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Equal(t, now.UnixMilli(), dst)

	// int64 (milliseconds) -> Timestamp (round-trip)
	src2 := reflect.ValueOf(dst)
	var dst2 timestamppb.Timestamp
	dstVal2 := reflect.ValueOf(&dst2).Elem()

	codec("CreatedAt", src2, dstVal2, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.True(t, now.Equal(dst2.AsTime()), "round-trip should preserve the time instant")
}

func TestNew_PassThrough(t *testing.T) {
	codec := New()

	src := reflect.ValueOf("hello")
	var dst string
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	codec("Name", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	assert.True(t, nextCalled, "next should be called for non-matching types")
}

func TestNew_NilPointer_Timestamp(t *testing.T) {
	codec := New()

	var src *timestamppb.Timestamp
	srcVal := reflect.ValueOf(src)
	var dst *time.Time
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", srcVal, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Nil(t, dst, "dst should remain nil when src is nil pointer")
}

func TestNew_NilPointer_Time(t *testing.T) {
	codec := New()

	var src *time.Time
	srcVal := reflect.ValueOf(src)
	var dst *timestamppb.Timestamp
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", srcVal, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Nil(t, dst, "dst should remain nil when src is nil pointer")
}

func TestNew_NilPointer_Int64(t *testing.T) {
	codec := New()

	var src *int64
	srcVal := reflect.ValueOf(src)
	var dst *timestamppb.Timestamp
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", srcVal, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Nil(t, dst, "dst should remain nil when src is nil pointer")
}
