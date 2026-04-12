// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package unixtime

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_TimeToInt64(t *testing.T) {
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	codec := New()

	src := reflect.ValueOf(ts)
	var dst int64
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, ts.Unix(), dst)
}

func TestNew_Int64ToTime(t *testing.T) {
	var unix int64 = 1750000000
	codec := New()

	src := reflect.ValueOf(unix)
	var dst time.Time
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, time.Unix(unix, 0), dst)
}

func TestNew_Pointer_TimeToInt64(t *testing.T) {
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	codec := New()

	src := reflect.ValueOf(&ts)
	var dst *int64
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	require.NotNil(t, dst)
	assert.Equal(t, ts.Unix(), *dst)
}

func TestNew_Pointer_Int64ToTime(t *testing.T) {
	var unix int64 = 1750000000
	codec := New()

	src := reflect.ValueOf(&unix)
	var dst *time.Time
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	require.NotNil(t, dst)
	assert.Equal(t, time.Unix(unix, 0), *dst)
}

func TestNew_IgnoreZero_Time(t *testing.T) {
	codec := New(WithIgnoreZero())

	src := reflect.ValueOf(time.Time{})
	var dst int64 = 42
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, int64(42), dst, "dst should remain unchanged when zero time is ignored")
}

func TestNew_IgnoreZero_Int64(t *testing.T) {
	codec := New(WithIgnoreZero())

	src := reflect.ValueOf(int64(0))
	original := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	dst := original
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, original, dst, "dst should remain unchanged when zero int64 is ignored")
}

func TestNew_Milliseconds(t *testing.T) {
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	codec := New(WithMilliseconds())

	// time.Time -> int64 (milliseconds)
	src := reflect.ValueOf(ts)
	var dst int64
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, ts.UnixMilli(), dst)

	// int64 (milliseconds) -> time.Time (round-trip)
	src2 := reflect.ValueOf(dst)
	var dst2 time.Time
	dstVal2 := reflect.ValueOf(&dst2).Elem()

	codec("CreatedAt", src2, dstVal2, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.True(t, ts.Equal(dst2), "round-trip should preserve the time instant")
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

func TestNew_NilPointer_Time(t *testing.T) {
	codec := New()

	var src *time.Time
	srcVal := reflect.ValueOf(src)
	var dst *int64
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", srcVal, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Nil(t, dst, "dst should remain nil when src is nil pointer")
}

func TestNew_NilPointer_Int64(t *testing.T) {
	codec := New()

	var src *int64
	srcVal := reflect.ValueOf(src)
	var dst *time.Time
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("CreatedAt", srcVal, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Nil(t, dst, "dst should remain nil when src is nil pointer")
}
