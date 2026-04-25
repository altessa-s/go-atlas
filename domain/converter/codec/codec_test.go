// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convcodec

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCodecsSet_Empty(t *testing.T) {
	s := NewCodecsSet()
	require.False(t, s.HasCodecs(), "expected no codecs")
}

func TestSet_Add(t *testing.T) {
	s := NewCodecsSet()
	c := Codec(func(string, reflect.Value, reflect.Value, CodecHandler) {})
	s.Add(c)
	require.True(t, s.HasCodecs(), "expected codecs after Add")
}

func TestSet_Run_NoCodecs(t *testing.T) {
	s := NewCodecsSet()
	called := false
	s.Run("f", reflect.ValueOf(1), reflect.ValueOf(2), func(_ string, _, _ reflect.Value) {
		called = true
	})
	require.True(t, called, "expected finishHandler to be called when no codecs")
}

func TestSet_Run_SingleCodec(t *testing.T) {
	codecCalled := false
	c := Codec(func(_ string, src, dst reflect.Value, next CodecHandler) {
		codecCalled = true
		next("", src, dst)
	})
	s := NewCodecsSet(c)

	finishCalled := false
	s.Run("f", reflect.ValueOf(1), reflect.ValueOf(2), func(_ string, _, _ reflect.Value) {
		finishCalled = true
	})
	require.True(t, codecCalled, "expected codec to be called")
	require.True(t, finishCalled, "expected finishHandler to be called")
}

func TestSet_Run_ChainOrder(t *testing.T) {
	var order []int
	c1 := Codec(func(_ string, src, dst reflect.Value, next CodecHandler) {
		order = append(order, 1)
		next("", src, dst)
	})
	c2 := Codec(func(_ string, src, dst reflect.Value, next CodecHandler) {
		order = append(order, 2)
		next("", src, dst)
	})
	s := NewCodecsSet(c1, c2)

	s.Run("f", reflect.ValueOf(1), reflect.ValueOf(2), func(_ string, _, _ reflect.Value) {
		order = append(order, 3)
	})

	require.Equal(t, []int{1, 2, 3}, order)
}

func TestSet_Run_CodecStopsChain(t *testing.T) {
	c1 := Codec(func(_ string, _, _ reflect.Value, _ CodecHandler) {
		// does NOT call next
	})
	c2 := Codec(func(_ string, _, _ reflect.Value, _ CodecHandler) {
		require.Fail(t, "c2 should not be called")
	})
	s := NewCodecsSet(c1, c2)

	s.Run("f", reflect.ValueOf(1), reflect.ValueOf(2), func(_ string, _, _ reflect.Value) {
		require.Fail(t, "finishHandler should not be called")
	})
}
