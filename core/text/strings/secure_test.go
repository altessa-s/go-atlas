// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestNewSecureString(t *testing.T) {
	ss := corestrings.NewSecureString("secret")
	require.NotNil(t, ss)
	defer ss.Clear()

	require.False(t, ss.IsEmpty(), "IsEmpty() should be false")
	require.Equal(t, 6, ss.Len())
}

func TestSecureString_String(t *testing.T) {
	ss := corestrings.NewSecureString("secret")
	defer ss.Clear()

	require.Equal(t, "secret", ss.String())
}

func TestSecureString_StringUnsafe(t *testing.T) {
	ss := corestrings.NewSecureString("secret")
	defer ss.Clear()

	require.Equal(t, "secret", ss.StringUnsafe())
}

func TestSecureString_Bytes(t *testing.T) {
	ss := corestrings.NewSecureString("abc")
	defer ss.Clear()

	require.Equal(t, "abc", string(ss.Bytes()))
}

func TestSecureString_BytesUnsafe(t *testing.T) {
	ss := corestrings.NewSecureString("abc")
	defer ss.Clear()

	require.Equal(t, "abc", string(ss.BytesUnsafe()))
}

func TestSecureString_Equal(t *testing.T) {
	a := corestrings.NewSecureString("same")
	b := corestrings.NewSecureString("same")
	c := corestrings.NewSecureString("diff")
	defer a.Clear()
	defer b.Clear()
	defer c.Clear()

	require.True(t, a.Equal(b), "Equal() should be true for same content")
	require.False(t, a.Equal(c), "Equal() should be false for different content")
}

func TestSecureString_Clear(t *testing.T) {
	ss := corestrings.NewSecureString("secret")
	ss.Clear()

	require.True(t, ss.IsEmpty(), "IsEmpty() should be true after Clear()")
	require.Equal(t, 0, ss.Len())
}

func TestSecureString_GoString(t *testing.T) {
	ss := corestrings.NewSecureString("secret")
	defer ss.Clear()

	require.Equal(t, "SecureString{<redacted>}", ss.GoString())
}

func TestSecureString_Empty(t *testing.T) {
	ss := corestrings.NewSecureString("")
	defer ss.Clear()

	require.True(t, ss.IsEmpty(), "IsEmpty() should be true for empty string")
}

func TestZeroBytes(t *testing.T) {
	b := []byte("secret")
	corestrings.ZeroBytes(b)
	for i, v := range b {
		require.Equal(t, byte(0), v, "ZeroBytes: byte[%d] should be 0", i)
	}
}
