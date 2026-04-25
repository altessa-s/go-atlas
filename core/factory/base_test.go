// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBase(t *testing.T) {
	b := NewBase(nil)
	require.NotNil(t, b.Logger(), "expected non-nil logger when nil passed")
}

func TestNewBase_WithLogger(t *testing.T) {
	l := slog.Default()
	b := NewBase(l)
	require.Equal(t, l, b.Logger(), "expected provided logger to be used")
}

func TestRequireDependency_NonNil(t *testing.T) {
	b := NewBase(nil)
	require.NoError(t, b.RequireDependency("value", "dep"))
}

func TestRequireDependency_Nil(t *testing.T) {
	b := NewBase(nil)
	err := b.RequireDependency(nil, "dep")
	require.Error(t, err, "expected error for nil dependency")
	require.Equal(t, "dep is required", err.Error())
}

func TestRequireAllDependencies(t *testing.T) {
	b := NewBase(nil)
	err := b.RequireAllDependencies(map[string]any{
		"a": "val",
		"b": nil,
	})
	require.Error(t, err, "expected error for nil dependency")
}

func TestRequireAllDependencies_AllPresent(t *testing.T) {
	b := NewBase(nil)
	err := b.RequireAllDependencies(map[string]any{
		"a": "val",
		"b": 42,
	})
	require.NoError(t, err)
}

func TestErrorf(t *testing.T) {
	b := NewBase(nil)
	err := b.Errorf("something %s", "failed")
	require.Equal(t, "something failed", err.Error())
}

func TestWrapError(t *testing.T) {
	b := NewBase(nil)
	cause := errors.New("root cause")
	err := b.WrapError(cause, "wrapping")
	require.ErrorIs(t, err, cause, "expected wrapped error to match cause")
	require.Equal(t, "wrapping: root cause", err.Error())
}

func TestWrapError_Nil(t *testing.T) {
	b := NewBase(nil)
	require.Nil(t, b.WrapError(nil, "msg"))
}
