// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/uniq"
)

func TestNewWithNoop(t *testing.T) {
	u := uniq.NewWithNoop()
	if u == nil {
		t.Fatal("NewWithNoop() returned nil")
	}
}

func TestNop_Add(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	if err := u.Add(ctx, "key1"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
}

func TestNop_Exist(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	_ = u.Add(ctx, "key1")
	exists, err := u.Exist(ctx, "key1")
	if err != nil {
		t.Fatalf("Exist() error = %v", err)
	}
	_ = exists // nop provider may or may not track
}

func TestNop_Remove(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	if err := u.Remove(ctx, "key1"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
}

func TestNop_Clear(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	if err := u.Clear(ctx); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
}

func TestNop_AddWithValue(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	if err := u.AddWithValue(ctx, "key1", "val1"); err != nil {
		t.Fatalf("AddWithValue() error = %v", err)
	}
}

func TestNop_GetValue(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	_ = u.AddWithValue(ctx, "key1", "val1")
	var out string
	err := u.GetValue(ctx, "key1", &out)
	// nop may not support GetValue, just ensure no panic
	_ = err
}

func TestAdd_EmptyKey_Nop(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	err := u.Add(ctx, "")
	if err == nil {
		t.Error("Add('') should return error")
	}
}
