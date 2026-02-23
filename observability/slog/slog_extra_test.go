// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog_test

import (
	"log/slog"
	"testing"
	"time"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

func TestFromContextOrDefault(t *testing.T) {
	// No logger in context — should return default
	l := slogx.FromContextOrDefault(t.Context())
	if l == nil {
		t.Fatal("FromContextOrDefault() returned nil")
	}

	// With logger in context
	ctx := slogx.ContextWithLogger(t.Context(), slog.Default())
	l2 := slogx.FromContextOrDefault(ctx)
	if l2 == nil {
		t.Fatal("FromContextOrDefault(with logger) returned nil")
	}
}

func TestContextWithLogger_DoesNotReplace(t *testing.T) {
	first := slog.Default().With("id", "first")
	ctx := slogx.ContextWithLogger(t.Context(), first)

	second := slog.Default().With("id", "second")
	ctx = slogx.ContextWithLogger(ctx, second)

	got := slogx.FromContext(ctx)
	// Should still be the first logger
	if got != first {
		t.Error("ContextWithLogger should not replace existing logger")
	}
}

func TestContextWithLogger_NilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil logger")
		}
	}()

	slogx.ContextWithLogger(t.Context(), nil)
}

func TestBuildLogger_NilBase(t *testing.T) {
	l := slogx.BuildLogger(t.Context(), nil)
	if l == nil {
		t.Fatal("BuildLogger(nil) returned nil")
	}
}

func TestBuildLogger_WithFields(t *testing.T) {
	fields := slogx.Fields{
		{Key: "service", Value: "test"},
	}
	ctx := slogx.InjectFields(t.Context(), fields)
	l := slogx.BuildLogger(ctx, slog.Default())
	if l == nil {
		t.Fatal("BuildLogger with fields returned nil")
	}
}

func TestInjectLogger(t *testing.T) {
	ctx := slogx.InjectLogger(t.Context(), slog.Default())
	l := slogx.FromContext(ctx)
	if l == nil {
		t.Error("InjectLogger should store logger in context")
	}
}

func TestFieldsFromContext_Empty(t *testing.T) {
	fields := slogx.FieldsFromContext(t.Context())
	if fields != nil {
		t.Errorf("FieldsFromContext(empty) = %v, want nil", fields)
	}
}

func TestInjectFields_AndRetrieve(t *testing.T) {
	f := slogx.Fields{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: 42},
	}
	ctx := slogx.InjectFields(t.Context(), f)

	got := slogx.FieldsFromContext(ctx)
	if len(got) != 2 {
		t.Fatalf("FieldsFromContext() len = %d, want 2", len(got))
	}
	if got[0].Key != "k1" || got[0].Value != "v1" {
		t.Errorf("field[0] = %v", got[0])
	}
}

func TestAppendField(t *testing.T) {
	ctx := slogx.InjectFields(t.Context(), slogx.Fields{})
	slogx.AppendField(ctx, "key", "val")

	got := slogx.FieldsFromContext(ctx)
	if len(got) != 1 || got[0].Key != "key" {
		t.Errorf("AppendField result = %v", got)
	}
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
	if len(got) != 2 {
		t.Errorf("AppendFields result len = %d, want 2", len(got))
	}
}

func TestFields_Append(t *testing.T) {
	f := slogx.Fields{}
	f = f.Append("a", 1)
	f = f.Append("b", 2)
	if len(f) != 2 {
		t.Errorf("len = %d, want 2", len(f))
	}
}

func TestFields_AppendFields(t *testing.T) {
	f := slogx.Fields{{Key: "a", Value: 1}}
	f = f.AppendFields(slogx.Fields{{Key: "b", Value: 2}})
	if len(f) != 2 {
		t.Errorf("len = %d, want 2", len(f))
	}
}

func TestFields_Delete(t *testing.T) {
	f := slogx.Fields{
		{Key: "a", Value: 1},
		{Key: "b", Value: 2},
		{Key: "c", Value: 3},
	}
	f = f.Delete("b")
	if len(f) != 2 {
		t.Errorf("len = %d, want 2", len(f))
	}
	for _, field := range f {
		if field.Key == "b" {
			t.Error("field 'b' should be deleted")
		}
	}
}

func TestFields_Delete_NotFound(t *testing.T) {
	f := slogx.Fields{{Key: "a", Value: 1}}
	f = f.Delete("nonexistent")
	if len(f) != 1 {
		t.Errorf("len = %d, want 1", len(f))
	}
}

func TestFields_Unique(t *testing.T) {
	f := slogx.Fields{
		{Key: "a", Value: 1},
		{Key: "a", Value: 2},
		{Key: "b", Value: 3},
	}
	u := f.Unique()
	if len(u) != 2 {
		t.Errorf("len = %d, want 2", len(u))
	}
	if u[0].Value != 1 {
		t.Error("Unique should keep first occurrence")
	}
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
	if count != 2 {
		t.Errorf("All() yielded %d, want 2", count)
	}
}

func TestFields_ToSlogArgs(t *testing.T) {
	f := slogx.Fields{
		{Key: "a", Value: 1},
		{Key: "b", Value: "two"},
	}
	args := f.ToSlogArgs()
	if len(args) != 4 {
		t.Errorf("ToSlogArgs() len = %d, want 4", len(args))
	}
	if args[0] != "a" || args[1] != 1 || args[2] != "b" || args[3] != "two" {
		t.Errorf("ToSlogArgs() = %v", args)
	}
}

func TestFields_ToSlogAttrs(t *testing.T) {
	f := slogx.Fields{
		{Key: "a", Value: 1},
	}
	attrs := f.ToSlogAttrs()
	if len(attrs) != 1 {
		t.Errorf("ToSlogAttrs() len = %d, want 1", len(attrs))
	}
	if attrs[0].Key != "a" {
		t.Errorf("attr key = %q", attrs[0].Key)
	}
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
	if len(attrs) != 16 {
		t.Errorf("FieldsToAttrs() len = %d, want 16", len(attrs))
	}
}

func TestFieldsToAttrs_Empty(t *testing.T) {
	attrs := slogx.FieldsToAttrs(nil)
	if attrs != nil {
		t.Error("FieldsToAttrs(nil) should return nil")
	}
}

func TestUseDiscardLoggerAsDefault(t *testing.T) {
	// Just ensure no panic
	slogx.UseDiscardLoggerAsDefault()
}

func TestSetGetLevel(t *testing.T) {
	slogx.SetLevel(slog.LevelWarn)
	got := slogx.GetLevel()
	if got != slog.LevelWarn {
		t.Errorf("GetLevel() = %v, want Warn", got)
	}
	// Reset
	slogx.SetLevel(slog.LevelInfo)
}
