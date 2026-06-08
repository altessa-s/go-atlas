// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redacted_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/redacted"
)

const (
	plain    = "super-secret"
	expected = "<redacted>"
)

func TestRedactedString_String(t *testing.T) {
	t.Parallel()

	s := redacted.RedactedString(plain)
	require.Equal(t, expected, s.String())
}

func TestRedactedString_FmtVerbs(t *testing.T) {
	t.Parallel()

	s := redacted.RedactedString(plain)

	cases := []struct {
		verb string
		want string
	}{
		{"%v", expected},
		{"%s", expected},
		{"%+v", expected},
		{"%q", `"` + expected + `"`},
		{"%#v", "RedactedString{" + expected + "}"},
	}
	for _, tc := range cases {
		t.Run(tc.verb, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, fmt.Sprintf(tc.verb, s))
		})
	}
}

func TestRedactedString_GoString(t *testing.T) {
	t.Parallel()

	s := redacted.RedactedString(plain)
	require.Equal(t, "RedactedString{"+expected+"}", s.GoString())
}

func TestRedactedString_LogValue(t *testing.T) {
	t.Parallel()

	s := redacted.RedactedString(plain)
	require.Equal(t, slog.StringValue(expected), s.LogValue())
}

func TestRedactedString_SlogTextHandler(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	s := redacted.RedactedString(plain)

	logger.Info("creds", "pwd", s)

	out := buf.String()
	require.Contains(t, out, "pwd="+expected)
	require.NotContains(t, out, plain)
}

func TestRedactedString_SlogJSONHandler(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	s := redacted.RedactedString(plain)

	logger.Info("creds", "pwd", s)

	out := buf.String()
	require.Contains(t, out, `"pwd":"`+expected+`"`)
	require.NotContains(t, out, plain)
}

func TestRedactedString_Expose(t *testing.T) {
	t.Parallel()

	require.Equal(t, plain, redacted.RedactedString(plain).Expose())
	require.Equal(t, "", redacted.RedactedString("").Expose())
}

func TestRedactedString_IsEmpty(t *testing.T) {
	t.Parallel()

	require.True(t, redacted.RedactedString("").IsEmpty())
	require.False(t, redacted.RedactedString("x").IsEmpty())
}

func TestRedactedString_IsZero(t *testing.T) {
	t.Parallel()

	require.True(t, redacted.RedactedString("").IsZero())
	require.False(t, redacted.RedactedString("x").IsZero())
}

func TestRedactedString_ZeroValue(t *testing.T) {
	t.Parallel()

	var s redacted.RedactedString
	require.True(t, s.IsEmpty())
	require.True(t, s.IsZero())
	require.Equal(t, "", s.Expose())
	require.Equal(t, expected, s.String())
}

func TestRedactedString_SecureString(t *testing.T) {
	t.Parallel()

	ss := redacted.RedactedString(plain).SecureString()
	require.NotNil(t, ss)
	require.Equal(t, plain, ss.String())
	ss.Clear()
}

func TestRedactedString_UnderlyingKindString(t *testing.T) {
	t.Parallel()

	// The reflect-based config loader, yaml.v3 scalar decoder, and Mongo
	// driver all rely on RedactedString having underlying kind string;
	// guard against an accidental redefinition (e.g., to a struct).
	var s redacted.RedactedString
	require.Equal(t, reflect.String, reflect.TypeOf(s).Kind())
}

func TestRedactedString_Comparable(t *testing.T) {
	t.Parallel()

	// Underlying string lets callers compare with untyped string constants —
	// useful in config validation ("if cfg.Token == \"\"").
	require.True(t, redacted.RedactedString("") == "")
	require.True(t, redacted.RedactedString("x") != "")
}
