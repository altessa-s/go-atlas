// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisbase_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/altessa-s/go-atlas/data/internal/redisbase"

	goredis "github.com/redis/go-redis/v9"
)

func setupBase(tb testing.TB, prefix string) (redisbase.Base, *miniredis.Miniredis) {
	tb.Helper()
	mr := miniredis.RunT(tb)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return redisbase.NewBase(client, prefix), mr
}

func TestNewBase(t *testing.T) {
	base, _ := setupBase(t, "test")
	if base.Client() == nil {
		t.Fatal("Client() should not be nil")
	}
	if base.Keys() == nil {
		t.Fatal("Keys() should not be nil")
	}
}

func TestNewBase_NilClient_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil client")
		}
	}()
	redisbase.NewBase(nil, "prefix")
}

func TestBase_Key(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		key    string
		want   string
	}{
		{"with prefix", "app", "user:1", "app:user:1"},
		{"empty prefix", "", "user:1", "user:1"},
		{"empty key", "app", "", "app:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, _ := setupBase(t, tt.prefix)
			if got := base.Key(tt.key); got != tt.want {
				t.Errorf("Key(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestBase_BuildKeys(t *testing.T) {
	base, _ := setupBase(t, "app")
	got := base.BuildKeys("k1", "k2", "k3")
	want := []string{"app:k1", "app:k2", "app:k3"}
	if len(got) != len(want) {
		t.Fatalf("BuildKeys() returned %d items, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("BuildKeys()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBase_Exists_True(t *testing.T) {
	base, mr := setupBase(t, "")
	ctx := t.Context()

	mr.Set("mykey", "val")

	exists, err := base.Exists(ctx, "mykey")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}
}

func TestBase_Exists_False(t *testing.T) {
	base, _ := setupBase(t, "")
	ctx := t.Context()

	exists, err := base.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if exists {
		t.Error("Exists() = true, want false")
	}
}

func TestBase_Exists_WithPrefix(t *testing.T) {
	base, mr := setupBase(t, "pfx")
	ctx := t.Context()

	mr.Set("pfx:mykey", "val")

	exists, err := base.Exists(ctx, "mykey")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if !exists {
		t.Error("Exists() with prefix = false, want true")
	}
}

func TestBase_Delete(t *testing.T) {
	base, mr := setupBase(t, "")
	ctx := t.Context()

	mr.Set("mykey", "val")

	err := base.Delete(ctx, "mykey")
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	if mr.Exists("mykey") {
		t.Error("key should have been deleted")
	}
}

func TestBase_Delete_NonExistent(t *testing.T) {
	base, _ := setupBase(t, "")
	ctx := t.Context()

	err := base.Delete(ctx, "missing")
	if err != nil {
		t.Errorf("Delete() on non-existent key should not error: %v", err)
	}
}

func TestBase_DeleteMany(t *testing.T) {
	base, mr := setupBase(t, "")
	ctx := t.Context()

	mr.Set("k1", "v1")
	mr.Set("k2", "v2")
	mr.Set("k3", "v3")

	err := base.DeleteMany(ctx, "k1", "k2", "k3")
	if err != nil {
		t.Fatalf("DeleteMany() error: %v", err)
	}

	for _, k := range []string{"k1", "k2", "k3"} {
		if mr.Exists(k) {
			t.Errorf("key %q should have been deleted", k)
		}
	}
}

func TestBase_DeleteMany_WithPrefix(t *testing.T) {
	base, mr := setupBase(t, "pfx")
	ctx := t.Context()

	mr.Set("pfx:k1", "v1")
	mr.Set("pfx:k2", "v2")

	err := base.DeleteMany(ctx, "k1", "k2")
	if err != nil {
		t.Fatalf("DeleteMany() error: %v", err)
	}

	for _, k := range []string{"pfx:k1", "pfx:k2"} {
		if mr.Exists(k) {
			t.Errorf("key %q should have been deleted", k)
		}
	}
}

func TestBase_Client(t *testing.T) {
	base, _ := setupBase(t, "")
	if base.Client() == nil {
		t.Error("Client() should not be nil")
	}
}

func TestBase_Keys_Accessor(t *testing.T) {
	base, _ := setupBase(t, "myprefix")
	kb := base.Keys()
	if kb == nil {
		t.Fatal("Keys() should not be nil")
	}
	if got := kb.Prefix(); got != "myprefix" {
		t.Errorf("Keys().Prefix() = %q, want %q", got, "myprefix")
	}
}
