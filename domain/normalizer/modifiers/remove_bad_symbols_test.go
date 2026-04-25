// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/normalizer/modifiers"
)

func TestRemoveBadSymbols_Clean(t *testing.T) {
	v := reflect.ValueOf("hello world")
	result := modifiers.RemoveBadSymbols(v, nil)
	require.Nil(t, result.Error)
	require.Equal(t, "hello world", result.Value.String())
}

func TestRemoveBadSymbols_WithControlChars(t *testing.T) {
	v := reflect.ValueOf("hello\x00world")
	result := modifiers.RemoveBadSymbols(v, nil)
	require.Nil(t, result.Error)
	got := result.Value.String()
	require.Equal(t, "helloworld", got)
}

func TestRemoveBadSymbols_Empty(t *testing.T) {
	v := reflect.ValueOf("")
	result := modifiers.RemoveBadSymbols(v, nil)
	require.Nil(t, result.Error)
}

func TestRemoveBadSymbols_Pointer(t *testing.T) {
	s := "test\x01value"
	v := reflect.ValueOf(&s)
	result := modifiers.RemoveBadSymbols(v, nil)
	require.Nil(t, result.Error)
}
