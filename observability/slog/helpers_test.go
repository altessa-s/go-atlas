// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"errors"
	"log/slog"
	"testing"
)

func TestError_NilReturnsEmpty(t *testing.T) {
	attr := Error(nil)
	if attr.Key != "" {
		t.Errorf("expected empty attr, got key=%q", attr.Key)
	}
}

func TestError_NonNil(t *testing.T) {
	attr := Error(errors.New("fail"))
	if attr.Key != ErrorKey {
		t.Errorf("Key = %q, want %q", attr.Key, ErrorKey)
	}
}

func TestString_Value(t *testing.T) {
	attr := String("name", "val")
	if attr.Key != "name" {
		t.Errorf("Key = %q", attr.Key)
	}
	if attr.Value.String() != "val" {
		t.Errorf("Value = %q", attr.Value.String())
	}
}

func TestString_EmptyReturnsEmpty(t *testing.T) {
	attr := String("name", "")
	if attr.Key != "" {
		t.Errorf("expected empty attr for empty string, got key=%q", attr.Key)
	}
}

func TestString_Pointer(t *testing.T) {
	v := "hello"
	attr := String("name", &v)
	if attr.Value.String() != "hello" {
		t.Errorf("Value = %q", attr.Value.String())
	}
}

func TestString_NilPointer(t *testing.T) {
	attr := String[*string]("name", nil)
	if attr.Key != "" {
		t.Errorf("expected empty attr for nil pointer, got key=%q", attr.Key)
	}
}

func TestInt_Value(t *testing.T) {
	attr := Int("count", 42)
	if attr.Key != "count" {
		t.Errorf("Key = %q", attr.Key)
	}
	if attr.Value.Int64() != 42 {
		t.Errorf("Value = %d", attr.Value.Int64())
	}
}

func TestInt_Pointer(t *testing.T) {
	v := 42
	attr := Int("count", &v)
	if attr.Value.Int64() != 42 {
		t.Errorf("Value = %d", attr.Value.Int64())
	}
}

func TestInt_NilPointer(t *testing.T) {
	attr := Int[*int]("count", nil)
	if attr.Key != "" {
		t.Errorf("expected empty attr for nil, got key=%q", attr.Key)
	}
}

func TestInt64_Value(t *testing.T) {
	attr := Int64("id", int64(100))
	if attr.Value.Int64() != 100 {
		t.Errorf("Value = %d", attr.Value.Int64())
	}
}

func TestInt64_Int32(t *testing.T) {
	attr := Int64("id", int32(50))
	if attr.Value.Int64() != 50 {
		t.Errorf("Value = %d", attr.Value.Int64())
	}
}

func TestInt64_NilPointers(t *testing.T) {
	attr64 := Int64[*int64]("id", nil)
	if attr64.Key != "" {
		t.Error("expected empty for nil *int64")
	}

	attr32 := Int64[*int32]("id", nil)
	if attr32.Key != "" {
		t.Error("expected empty for nil *int32")
	}
}

func TestModule(t *testing.T) {
	attr := Module("http-server")
	if attr.Key != ModuleKey {
		t.Errorf("Key = %q, want %q", attr.Key, ModuleKey)
	}
	if attr.Value.String() != "http-server" {
		t.Errorf("Value = %q", attr.Value.String())
	}
}

func TestModuleM(t *testing.T) {
	args := ModuleM("auth", "cache")
	if len(args) != 2 {
		t.Fatalf("len = %d", len(args))
	}
	for _, arg := range args {
		a, ok := arg.(slog.Attr)
		if !ok {
			t.Errorf("expected slog.Attr, got %T", arg)
		}
		if a.Key != ModuleKey {
			t.Errorf("Key = %q", a.Key)
		}
	}
}

func TestSetGetLevel(t *testing.T) {
	SetLevel(slog.LevelWarn)
	if GetLevel() != slog.LevelWarn {
		t.Errorf("GetLevel() = %v", GetLevel())
	}
	SetLevel(slog.LevelInfo) // restore
}

func TestMaskingReplaceAttr(t *testing.T) {
	tests := []struct {
		name      string
		sensitive []string
		key       string
		wantMask  bool
	}{
		{"sensitive key", []string{"password"}, "password", true},
		{"case insensitive", []string{"Password"}, "password", true},
		{"not sensitive", []string{"password"}, "username", false},
		{"empty list", nil, "password", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := MaskingReplaceAttr(tt.sensitive, "***")
			attr := slog.String(tt.key, "secret")
			result := fn(nil, attr)
			if tt.wantMask && result.Value.String() != "***" {
				t.Errorf("expected masked, got %q", result.Value.String())
			}
			if !tt.wantMask && result.Value.String() == "***" {
				t.Error("should not be masked")
			}
		})
	}
}
