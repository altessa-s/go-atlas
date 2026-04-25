// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convcodec_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
)

func TestNewCodecsSet_NilCodecs(t *testing.T) {
	s := convcodec.NewCodecsSet()
	if s.HasCodecs() {
		require.Fail(t, "empty set should not have codecs")
	}
}

func TestSet_HasCodecs(t *testing.T) {
	s := convcodec.NewCodecsSet()
	require.False(t, s.HasCodecs(), "empty set HasCodecs() = true")

	s.Add(func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		next(fieldName, src, dst)
	})
	require.True(t, s.HasCodecs(), "set with codec HasCodecs() = false")
}

func TestSet_Run_FinishHandlerCalled(t *testing.T) {
	s := convcodec.NewCodecsSet()

	var called bool
	s.Run("field", reflect.ValueOf(0), reflect.ValueOf(0), func(fieldName string, src, dst reflect.Value) {
		called = true
	})
	require.True(t, called, "finishHandler not called when no codecs")
}

func TestSet_Run_CodecHandlesConversion(t *testing.T) {
	var codecFieldName string
	codec := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		codecFieldName = fieldName
		// Handle conversion, don't call next
	}

	s := convcodec.NewCodecsSet(codec)

	finishCalled := false
	s.Run("myField", reflect.ValueOf(1), reflect.ValueOf(2), func(fieldName string, src, dst reflect.Value) {
		finishCalled = true
	})

	require.Equal(t, "myField", codecFieldName)
	require.False(t, finishCalled, "finishHandler should not be called when codec handles conversion")
}

func TestSet_Run_CodecPassesToNext(t *testing.T) {
	passthrough := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		next(fieldName, src, dst)
	}

	s := convcodec.NewCodecsSet(passthrough)

	var finishCalled bool
	s.Run("f", reflect.ValueOf(0), reflect.ValueOf(0), func(fieldName string, src, dst reflect.Value) {
		finishCalled = true
	})
	require.True(t, finishCalled, "finishHandler should be called when codec passes through")
}

func TestSet_Run_MultipleCodecsChain(t *testing.T) {
	var order []int

	codec1 := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		order = append(order, 1)
		next(fieldName, src, dst)
	}
	codec2 := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		order = append(order, 2)
		next(fieldName, src, dst)
	}
	codec3 := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		order = append(order, 3)
		next(fieldName, src, dst)
	}

	s := convcodec.NewCodecsSet(codec1, codec2, codec3)

	s.Run("f", reflect.ValueOf(0), reflect.ValueOf(0), func(fieldName string, src, dst reflect.Value) {
		order = append(order, 99)
	})

	require.Equal(t, []int{1, 2, 3, 99}, order)
}

func TestSet_Run_CodecStopsChainMidway(t *testing.T) {
	var order []int

	codec1 := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		order = append(order, 1)
		next(fieldName, src, dst)
	}
	codec2 := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		order = append(order, 2)
		// Stop chain, don't call next
	}
	codec3 := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		order = append(order, 3)
		next(fieldName, src, dst)
	}

	s := convcodec.NewCodecsSet(codec1, codec2, codec3)

	s.Run("f", reflect.ValueOf(0), reflect.ValueOf(0), func(fieldName string, src, dst reflect.Value) {
		order = append(order, 99)
	})

	require.Equal(t, []int{1, 2}, order)
}

func TestSet_Add_Chaining(t *testing.T) {
	noop := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		next(fieldName, src, dst)
	}

	s := convcodec.NewCodecsSet().Add(noop).Add(noop, noop)
	require.True(t, s.HasCodecs(), "set should have codecs after chained Add")
}
