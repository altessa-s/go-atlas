// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo

import (
	"os"
	"testing"
)

func TestEnv(t *testing.T) {
	t.Setenv("TEST_APPINFO_KEY", "value123")
	if got := Env("TEST_APPINFO_KEY"); got != "value123" {
		t.Fatalf("got %q, want %q", got, "value123")
	}
	if got := Env("TEST_APPINFO_MISSING"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("TEST_APPINFO_ENVOR", "set")
	if got := EnvOr("TEST_APPINFO_ENVOR", "default"); got != "set" {
		t.Fatalf("got %q, want %q", got, "set")
	}
	if got := EnvOr("TEST_APPINFO_MISSING_ENVOR", "fallback"); got != "fallback" {
		t.Fatalf("got %q, want %q", got, "fallback")
	}
}

func TestEnvCached(t *testing.T) {
	ClearEnvCache()
	t.Setenv("TEST_APPINFO_CACHED", "cached_val")

	v1 := EnvCached("TEST_APPINFO_CACHED")
	if v1 != "cached_val" {
		t.Fatalf("got %q, want %q", v1, "cached_val")
	}

	// Change env but cache should still return old value
	os.Setenv("TEST_APPINFO_CACHED", "new_val")
	v2 := EnvCached("TEST_APPINFO_CACHED")
	if v2 != "cached_val" {
		t.Fatalf("got %q, want cached %q", v2, "cached_val")
	}

	// Restore
	os.Setenv("TEST_APPINFO_CACHED", "cached_val")
}

func TestClearEnvCache(t *testing.T) {
	ClearEnvCache()
	t.Setenv("TEST_APPINFO_CLEAR", "original")
	_ = EnvCached("TEST_APPINFO_CLEAR")

	ClearEnvCache()
	os.Setenv("TEST_APPINFO_CLEAR", "updated")
	if got := EnvCached("TEST_APPINFO_CLEAR"); got != "updated" {
		t.Fatalf("after clear, got %q, want %q", got, "updated")
	}

	// Restore
	os.Setenv("TEST_APPINFO_CLEAR", "original")
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		home bool // whether result should start with home dir
	}{
		{"tilde path", "~/Documents", true},
		{"absolute path", "/tmp/file", false},
		{"relative path", "relative/path", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandPath(tt.path)
			if tt.home {
				home := HomeDir()
				if home != "" && got[:len(home)] != home {
					t.Errorf("got %q, expected to start with %q", got, home)
				}
			} else if got != tt.path {
				t.Errorf("got %q, want %q", got, tt.path)
			}
		})
	}
}

func TestHomeDir(t *testing.T) {
	home := HomeDir()
	if home == "" {
		t.Skip("HOME not set")
	}
	if home[0] != '/' {
		t.Errorf("expected absolute path, got %q", home)
	}
}
