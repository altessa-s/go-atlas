// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"testing"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestNewSecureString(t *testing.T) {
	ss := corestrings.NewSecureString("secret")
	if ss == nil {
		t.Fatal("NewSecureString() returned nil")
	}
	defer ss.Clear()

	if ss.IsEmpty() {
		t.Error("IsEmpty() should be false")
	}
	if ss.Len() != 6 {
		t.Errorf("Len() = %d, want 6", ss.Len())
	}
}

func TestSecureString_String(t *testing.T) {
	ss := corestrings.NewSecureString("secret")
	defer ss.Clear()

	if got := ss.String(); got != "secret" {
		t.Errorf("String() = %q, want secret", got)
	}
}

func TestSecureString_StringUnsafe(t *testing.T) {
	ss := corestrings.NewSecureString("secret")
	defer ss.Clear()

	if got := ss.StringUnsafe(); got != "secret" {
		t.Errorf("StringUnsafe() = %q, want secret", got)
	}
}

func TestSecureString_Bytes(t *testing.T) {
	ss := corestrings.NewSecureString("abc")
	defer ss.Clear()

	b := ss.Bytes()
	if string(b) != "abc" {
		t.Errorf("Bytes() = %q, want abc", b)
	}
}

func TestSecureString_BytesUnsafe(t *testing.T) {
	ss := corestrings.NewSecureString("abc")
	defer ss.Clear()

	b := ss.BytesUnsafe()
	if string(b) != "abc" {
		t.Errorf("BytesUnsafe() = %q, want abc", b)
	}
}

func TestSecureString_Equal(t *testing.T) {
	a := corestrings.NewSecureString("same")
	b := corestrings.NewSecureString("same")
	c := corestrings.NewSecureString("diff")
	defer a.Clear()
	defer b.Clear()
	defer c.Clear()

	if !a.Equal(b) {
		t.Error("Equal() should be true for same content")
	}
	if a.Equal(c) {
		t.Error("Equal() should be false for different content")
	}
}

func TestSecureString_Clear(t *testing.T) {
	ss := corestrings.NewSecureString("secret")
	ss.Clear()

	if !ss.IsEmpty() {
		t.Error("IsEmpty() should be true after Clear()")
	}
	if ss.Len() != 0 {
		t.Errorf("Len() = %d after Clear(), want 0", ss.Len())
	}
}

func TestSecureString_GoString(t *testing.T) {
	ss := corestrings.NewSecureString("secret")
	defer ss.Clear()

	got := ss.GoString()
	if got != "SecureString{<redacted>}" {
		t.Errorf("GoString() = %q, want SecureString{<redacted>}", got)
	}
}

func TestSecureString_Empty(t *testing.T) {
	ss := corestrings.NewSecureString("")
	defer ss.Clear()

	if !ss.IsEmpty() {
		t.Error("IsEmpty() should be true for empty string")
	}
}

func TestZeroBytes(t *testing.T) {
	b := []byte("secret")
	corestrings.ZeroBytes(b)
	for i, v := range b {
		if v != 0 {
			t.Errorf("ZeroBytes: byte[%d] = %d, want 0", i, v)
		}
	}
}
