// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

func TestFromContextOrDefault(t *testing.T) {
	// No logger in context — should return default
	l := slogx.FromContextOrDefault(t.Context())
	require.NotNil(t, l)

	// With logger in context
	ctx := slogx.ContextWithLogger(t.Context(), slog.Default())
	l2 := slogx.FromContextOrDefault(ctx)
	require.NotNil(t, l2)
}

func TestContextWithLogger_DoesNotReplace(t *testing.T) {
	first := slog.Default().With("id", "first")
	ctx := slogx.ContextWithLogger(t.Context(), first)

	second := slog.Default().With("id", "second")
	ctx = slogx.ContextWithLogger(ctx, second)

	got := slogx.FromContext(ctx)
	// Should still be the first logger
	require.Same(t, first, got, "ContextWithLogger should not replace existing logger")
}

func TestContextWithLogger_NilPanics(t *testing.T) {
	require.Panics(t, func() {
		slogx.ContextWithLogger(t.Context(), nil)
	})
}

func TestBuildLogger_NilBase(t *testing.T) {
	l := slogx.BuildLogger(t.Context(), nil)
	require.NotNil(t, l)
}

func TestBuildLogger_WithFields(t *testing.T) {
	fields := slogx.Fields{
		{Key: "service", Value: "test"},
	}
	ctx := slogx.InjectFields(t.Context(), fields)
	l := slogx.BuildLogger(ctx, slog.Default())
	require.NotNil(t, l)
}

func TestInjectLogger(t *testing.T) {
	ctx := slogx.InjectLogger(t.Context(), slog.Default())
	l := slogx.FromContext(ctx)
	require.NotNil(t, l, "InjectLogger should store logger in context")
}

func TestFieldsFromContext_Empty(t *testing.T) {
	fields := slogx.FieldsFromContext(t.Context())
	require.Nil(t, fields)
}

func TestInjectFields_AndRetrieve(t *testing.T) {
	f := slogx.Fields{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: 42},
	}
	ctx := slogx.InjectFields(t.Context(), f)

	got := slogx.FieldsFromContext(ctx)
	require.Len(t, got, 2)
	require.Equal(t, "k1", got[0].Key)
	require.Equal(t, "v1", got[0].Value)
}

func TestAppendField(t *testing.T) {
	ctx := slogx.InjectFields(t.Context(), slogx.Fields{})
	slogx.AppendField(ctx, "key", "val")

	got := slogx.FieldsFromContext(ctx)
	require.Len(t, got, 1)
	require.Equal(t, "key", got[0].Key)
}

func TestAppendField_NoFields(t *testing.T) {
	// No fields in context — should not panic
	slogx.AppendField(t.Context(), "key", "val")
}

func TestAppendFields(t *testing.T) {
	ctx := slogx.InjectFields(t.Context(), slogx.Fields{
		{Key: "existing", Value: "yes"},
	})
	slogx.AppendFields(ctx, slogx.Fields{
		{Key: "new", Value: "yes"},
	})

	got := slogx.FieldsFromContext(ctx)
	require.Len(t, got, 2)
}

func TestFields_Append(t *testing.T) {
	f := slogx.Fields{}
	f = f.Append("a", 1)
	f = f.Append("b", 2)
	require.Len(t, f, 2)
}

func TestFields_AppendFields(t *testing.T) {
	f := slogx.Fields{{Key: "a", Value: 1}}
	f = f.AppendFields(slogx.Fields{{Key: "b", Value: 2}})
	require.Len(t, f, 2)
}

func TestFields_Delete(t *testing.T) {
	f := slogx.Fields{
		{Key: "a", Value: 1},
		{Key: "b", Value: 2},
		{Key: "c", Value: 3},
	}
	f = f.Delete("b")
	require.Len(t, f, 2)
	for _, field := range f {
		require.NotEqual(t, "b", field.Key, "field 'b' should be deleted")
	}
}

func TestFields_Delete_NotFound(t *testing.T) {
	f := slogx.Fields{{Key: "a", Value: 1}}
	f = f.Delete("nonexistent")
	require.Len(t, f, 1)
}

func TestFields_Unique(t *testing.T) {
	f := slogx.Fields{
		{Key: "a", Value: 1},
		{Key: "a", Value: 2},
		{Key: "b", Value: 3},
	}
	u := f.Unique()
	require.Len(t, u, 2)
	require.Equal(t, 1, u[0].Value, "Unique should keep first occurrence")
}

func TestFields_All(t *testing.T) {
	f := slogx.Fields{
		{Key: "a", Value: 1},
		{Key: "b", Value: 2},
	}

	count := 0
	for k, v := range f.All() {
		_ = k
		_ = v
		count++
	}
	require.Equal(t, 2, count)
}

func TestFields_ToSlogArgs(t *testing.T) {
	f := slogx.Fields{
		{Key: "a", Value: 1},
		{Key: "b", Value: "two"},
	}
	args := f.ToSlogArgs()
	require.Len(t, args, 4)
	require.Equal(t, "a", args[0])
	require.Equal(t, 1, args[1])
	require.Equal(t, "b", args[2])
	require.Equal(t, "two", args[3])
}

func TestFields_ToSlogAttrs(t *testing.T) {
	f := slogx.Fields{
		{Key: "a", Value: 1},
	}
	attrs := f.ToSlogAttrs()
	require.Len(t, attrs, 1)
	require.Equal(t, "a", attrs[0].Key)
}

func TestFieldsToAttrs_TypeConversions(t *testing.T) {
	f := slogx.Fields{
		{Key: "str", Value: "hello"},
		{Key: "int", Value: 42},
		{Key: "int8", Value: int8(8)},
		{Key: "int16", Value: int16(16)},
		{Key: "int32", Value: int32(32)},
		{Key: "int64", Value: int64(64)},
		{Key: "uint", Value: uint(10)},
		{Key: "uint8", Value: uint8(8)},
		{Key: "uint16", Value: uint16(16)},
		{Key: "uint32", Value: uint32(32)},
		{Key: "uint64", Value: uint64(64)},
		{Key: "float32", Value: float32(3.14)},
		{Key: "float64", Value: float64(2.71)},
		{Key: "bool", Value: true},
		{Key: "duration", Value: time.Second},
		{Key: "time", Value: time.Now()},
	}

	attrs := slogx.FieldsToAttrs(f)
	require.Len(t, attrs, 16)
}

func TestFieldsToAttrs_Empty(t *testing.T) {
	attrs := slogx.FieldsToAttrs(nil)
	require.Nil(t, attrs)
}

func TestUseDiscardLoggerAsDefault(t *testing.T) {
	// Just ensure no panic
	slogx.UseDiscardLoggerAsDefault()
}

func TestSetGetLevel(t *testing.T) {
	slogx.SetLevel(slog.LevelWarn)
	got := slogx.GetLevel()
	require.Equal(t, slog.LevelWarn, got)
	// Reset
	slogx.SetLevel(slog.LevelInfo)
}
