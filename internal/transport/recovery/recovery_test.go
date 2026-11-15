// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPanicError_Error(t *testing.T) {
	tests := []struct {
		name  string
		panic any
		want  string
	}{
		{"string", "something broke", "panic: something broke"},
		{"int", 42, "panic: 42"},
		{"nil", nil, "panic: <nil>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pe := &PanicError{Panic: tt.panic}
			got := pe.Error()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNewPanicError(t *testing.T) {
	pe := NewPanicError("test panic", 0)
	require.NotNil(t, pe)
	require.Equal(t, "test panic", pe.Panic)
	require.NotEmpty(t, pe.Frames)
	// First frame should be this test function
	require.True(t, strings.Contains(pe.Frames[0].Function, "TestNewPanicError"))
}

func TestStackTrace(t *testing.T) {
	frames := StackTrace(0)
	require.NotEmpty(t, frames)
	require.True(t, strings.Contains(frames[0].Function, "TestStackTrace"))
	require.Greater(t, frames[0].Line, 0, "first frame line should be > 0")
	require.NotEqual(t, "", frames[0].File)
}

func TestFrames_String(t *testing.T) {
	tests := []struct {
		name   string
		frames Frames
		want   string
	}{
		{"empty", Frames{}, ""},
		{"single", Frames{{File: "a.go", Line: 10, Function: "Foo"}}, "a.go:10 Foo"},
		{"multiple", Frames{
			{File: "a.go", Line: 10, Function: "Foo"},
			{File: "b.go", Line: 20, Function: "Bar"},
		}, "a.go:10 Foo\nb.go:20 Bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.frames.String()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFrames_MarshalJSON(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		data, err := json.Marshal(Frames{})
		require.NoError(t, err)
		require.Equal(t, "[]", string(data))
	})

	t.Run("with_frames", func(t *testing.T) {
		frames := Frames{{File: "a.go", Line: 10, Function: "Foo"}}
		data, err := json.Marshal(frames)
		require.NoError(t, err)
		var result []Frame
		require.NoError(t, json.Unmarshal(data, &result))
		require.Len(t, result, 1)
		require.Equal(t, "a.go", result[0].File)
	})
}

func TestShortname(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/foo/bar.Func", "bar.Func"},
		{"main.Func", "main.Func"},
		{"Func", "Func"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := shortname(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}
