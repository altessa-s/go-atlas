// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/uniq/providers/noop"
)

func TestNew(t *testing.T) {
	p := noop.New()
	if p == nil {
		t.Fatal("New() returned nil")
	}
}

func TestProvider_Add(t *testing.T) {
	p := noop.New()
	if err := p.Add(t.Context(), "key"); err != nil {
		t.Errorf("Add() error: %v", err)
	}
}

func TestProvider_AddWithValue(t *testing.T) {
	p := noop.New()
	if err := p.AddWithValue(t.Context(), "key", []byte("val")); err != nil {
		t.Errorf("AddWithValue() error: %v", err)
	}
}

func TestProvider_Exist(t *testing.T) {
	p := noop.New()
	exists, err := p.Exist(t.Context(), "key")
	if err != nil {
		t.Errorf("Exist() error: %v", err)
	}
	if exists {
		t.Error("Exist() should return false for noop")
	}
}

func TestProvider_GetValue(t *testing.T) {
	p := noop.New()
	val, err := p.GetValue(t.Context(), "key")
	if err != nil {
		t.Errorf("GetValue() error: %v", err)
	}
	if val != nil {
		t.Errorf("GetValue() = %v, want nil", val)
	}
}

func TestProvider_Remove(t *testing.T) {
	p := noop.New()
	if err := p.Remove(t.Context(), "key"); err != nil {
		t.Errorf("Remove() error: %v", err)
	}
}

func TestProvider_Clear(t *testing.T) {
	p := noop.New()
	if err := p.Clear(t.Context()); err != nil {
		t.Errorf("Clear() error: %v", err)
	}
}

func TestProvider_FullLifecycle(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	_ = p.Add(ctx, "key1")
	_ = p.AddWithValue(ctx, "key2", []byte("data"))

	exists, _ := p.Exist(ctx, "key1")
	if exists {
		t.Error("noop Exist() should always be false")
	}

	val, _ := p.GetValue(ctx, "key2")
	if val != nil {
		t.Error("noop GetValue() should always be nil")
	}

	_ = p.Remove(ctx, "key1")
	_ = p.Clear(ctx)
}
