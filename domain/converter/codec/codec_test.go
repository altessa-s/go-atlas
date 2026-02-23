// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convcodec

import (
	"reflect"
	"testing"
)

func TestNewCodecsSet_Empty(t *testing.T) {
	s := NewCodecsSet()
	if s.HasCodecs() {
		t.Fatal("expected no codecs")
	}
}

func TestSet_Add(t *testing.T) {
	s := NewCodecsSet()
	c := Codec(func(string, reflect.Value, reflect.Value, CodecHandler) {})
	s.Add(c)
	if !s.HasCodecs() {
		t.Fatal("expected codecs after Add")
	}
}

func TestSet_Run_NoCodecs(t *testing.T) {
	s := NewCodecsSet()
	called := false
	s.Run("f", reflect.ValueOf(1), reflect.ValueOf(2), func(_ string, _, _ reflect.Value) {
		called = true
	})
	if !called {
		t.Fatal("expected finishHandler to be called when no codecs")
	}
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
	if !codecCalled {
		t.Fatal("expected codec to be called")
	}
	if !finishCalled {
		t.Fatal("expected finishHandler to be called")
	}
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

	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("expected order [1,2,3], got %v", order)
	}
}

func TestSet_Run_CodecStopsChain(t *testing.T) {
	c1 := Codec(func(_ string, _, _ reflect.Value, _ CodecHandler) {
		// does NOT call next
	})
	c2 := Codec(func(_ string, _, _ reflect.Value, _ CodecHandler) {
		t.Fatal("c2 should not be called")
	})
	s := NewCodecsSet(c1, c2)

	s.Run("f", reflect.ValueOf(1), reflect.ValueOf(2), func(_ string, _, _ reflect.Value) {
		t.Fatal("finishHandler should not be called")
	})
}
