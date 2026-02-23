// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"log/slog"
	"testing"
)

func TestNewBase(t *testing.T) {
	b := NewBase(nil)
	if b.Logger() == nil {
		t.Fatal("expected non-nil logger when nil passed")
	}
}

func TestNewBase_WithLogger(t *testing.T) {
	l := slog.Default()
	b := NewBase(l)
	if b.Logger() != l {
		t.Fatal("expected provided logger to be used")
	}
}

func TestRequireDependency_NonNil(t *testing.T) {
	b := NewBase(nil)
	if err := b.RequireDependency("value", "dep"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireDependency_Nil(t *testing.T) {
	b := NewBase(nil)
	err := b.RequireDependency(nil, "dep")
	if err == nil {
		t.Fatal("expected error for nil dependency")
	}
	if got := err.Error(); got != "dep is required" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireAllDependencies(t *testing.T) {
	b := NewBase(nil)
	err := b.RequireAllDependencies(map[string]any{
		"a": "val",
		"b": nil,
	})
	if err == nil {
		t.Fatal("expected error for nil dependency")
	}
}

func TestRequireAllDependencies_AllPresent(t *testing.T) {
	b := NewBase(nil)
	err := b.RequireAllDependencies(map[string]any{
		"a": "val",
		"b": 42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestErrorf(t *testing.T) {
	b := NewBase(nil)
	err := b.Errorf("something %s", "failed")
	if got := err.Error(); got != "something failed" {
		t.Fatalf("got %q", got)
	}
}

func TestWrapError(t *testing.T) {
	b := NewBase(nil)
	cause := errors.New("root cause")
	err := b.WrapError(cause, "wrapping")
	if !errors.Is(err, cause) {
		t.Fatal("expected wrapped error to match cause")
	}
	if got := err.Error(); got != "wrapping: root cause" {
		t.Fatalf("got %q", got)
	}
}

func TestWrapError_Nil(t *testing.T) {
	b := NewBase(nil)
	if err := b.WrapError(nil, "msg"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
