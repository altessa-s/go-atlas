// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pbwrap_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/converter/codec/pbwrap"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestNew_StringToWrapper(t *testing.T) {
	codec := pbwrap.New()

	src := reflect.ValueOf("hello")
	var dst *wrapperspb.StringValue
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Name", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	require.NotNil(t, dst)
	assert.Equal(t, "hello", dst.Value)
}

func TestNew_WrapperToString(t *testing.T) {
	codec := pbwrap.New()

	src := reflect.ValueOf(&wrapperspb.StringValue{Value: "hello"})
	var dst string
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Name", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, "hello", dst)
}

func TestNew_PassThrough(t *testing.T) {
	codec := pbwrap.New()

	src := reflect.ValueOf("hello")
	var dst string
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	codec("Name", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	assert.True(t, nextCalled, "next should be called for non-wrapper types")
}

func TestNew_IgnoreZeroValues(t *testing.T) {
	codec := pbwrap.New(pbwrap.WithIgnoreZeroValues())

	src := reflect.ValueOf("")
	var dst *wrapperspb.StringValue
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Name", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Nil(t, dst, "dst should remain nil when zero value is ignored")
}

func TestNew_IgnoreZeroWrappers(t *testing.T) {
	codec := pbwrap.New(pbwrap.WithIgnoreZeroWrappers())

	src := reflect.ValueOf(&wrapperspb.StringValue{Value: ""})
	var dst string
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Name", src, dstVal, func(_ string, _, _ reflect.Value) {
		require.Fail(t, "next should not be called")
	})

	assert.Equal(t, "", dst, "dst should remain zero when zero wrapper is ignored")
}
