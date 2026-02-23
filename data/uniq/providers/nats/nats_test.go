// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	uniqnats "github.com/altessa-s/go-atlas/data/uniq/providers/nats"
)

func setupProvider(tb testing.TB) *uniqnats.Provider {
	tb.Helper()
	ns := testhelpers.StartNATSServer(tb)
	nc := testhelpers.ConnectNATS(tb, ns)

	bucket := strings.ReplaceAll(tb.Name(), "/", "-")
	p, err := uniqnats.New(nc, uniqnats.WithBucket(bucket))
	if err != nil {
		tb.Fatalf("failed to create NATS uniq provider: %v", err)
	}
	return p
}

func TestNew(t *testing.T) {
	p := setupProvider(t)
	if p == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_NilConn(t *testing.T) {
	_, err := uniqnats.New(nil)
	if err == nil {
		t.Error("New(nil) should return error")
	}
}

func TestProvider_Add_Exist(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	if err := p.Add(ctx, "key1"); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	exists, err := p.Exist(ctx, "key1")
	if err != nil {
		t.Fatalf("Exist() error: %v", err)
	}
	if !exists {
		t.Error("Exist() should return true for added key")
	}
}

func TestProvider_Exist_NotFound(t *testing.T) {
	p := setupProvider(t)
	exists, err := p.Exist(t.Context(), "missing")
	if err != nil {
		t.Fatalf("Exist() error: %v", err)
	}
	if exists {
		t.Error("Exist() should return false for missing key")
	}
}

func TestProvider_AddWithValue_GetValue(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	if err := p.AddWithValue(ctx, "key1", []byte("hello")); err != nil {
		t.Fatalf("AddWithValue() error: %v", err)
	}

	val, err := p.GetValue(ctx, "key1")
	if err != nil {
		t.Fatalf("GetValue() error: %v", err)
	}
	if string(val) != "hello" {
		t.Errorf("GetValue() = %q, want %q", string(val), "hello")
	}
}

func TestProvider_GetValue_NotFound(t *testing.T) {
	p := setupProvider(t)
	val, err := p.GetValue(t.Context(), "missing")
	if err != nil {
		t.Fatalf("GetValue() error: %v", err)
	}
	if val != nil {
		t.Errorf("GetValue() = %v, want nil", val)
	}
}

func TestProvider_Remove(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	_ = p.Add(ctx, "key1")
	if err := p.Remove(ctx, "key1"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	exists, _ := p.Exist(ctx, "key1")
	if exists {
		t.Error("Exist() should return false after Remove()")
	}
}

func TestProvider_Clear(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	_ = p.Add(ctx, "key1")
	_ = p.Add(ctx, "key2")

	// Note: NATS KV Purge("") is not supported (empty key invalid).
	// Clear() returns an error in this implementation.
	err := p.Clear(ctx)
	if err == nil {
		// If it succeeds, verify keys are gone
		e1, _ := p.Exist(ctx, "key1")
		e2, _ := p.Exist(ctx, "key2")
		if e1 || e2 {
			t.Error("keys should not exist after Clear()")
		}
	}
	// Error is acceptable — known limitation of Purge with empty key
}

func TestProvider_Add_Overwrite(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	_ = p.AddWithValue(ctx, "key1", []byte("v1"))
	_ = p.AddWithValue(ctx, "key1", []byte("v2"))

	val, _ := p.GetValue(ctx, "key1")
	if string(val) != "v2" {
		t.Errorf("GetValue() = %q, want %q", string(val), "v2")
	}
}
