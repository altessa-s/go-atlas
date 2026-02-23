// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"encoding/json"
	"strings"
	"testing"
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
			if got := pe.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewPanicError(t *testing.T) {
	pe := NewPanicError("test panic", 0)
	if pe == nil {
		t.Fatal("NewPanicError returned nil")
	}
	if pe.Panic != "test panic" {
		t.Fatalf("Panic = %v, want %q", pe.Panic, "test panic")
	}
	if len(pe.Frames) == 0 {
		t.Fatal("Frames is empty")
	}
	// First frame should be this test function
	if !strings.Contains(pe.Frames[0].Function, "TestNewPanicError") {
		t.Fatalf("first frame function = %q, want containing TestNewPanicError", pe.Frames[0].Function)
	}
}

func TestStackTrace(t *testing.T) {
	frames := StackTrace(0)
	if len(frames) == 0 {
		t.Fatal("StackTrace returned empty frames")
	}
	if !strings.Contains(frames[0].Function, "TestStackTrace") {
		t.Fatalf("first frame function = %q, want containing TestStackTrace", frames[0].Function)
	}
	if frames[0].Line <= 0 {
		t.Fatalf("first frame line = %d, want > 0", frames[0].Line)
	}
	if frames[0].File == "" {
		t.Fatal("first frame file is empty")
	}
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
			if got := tt.frames.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFrames_MarshalJSON(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		data, err := json.Marshal(Frames{})
		if err != nil {
			t.Fatalf("MarshalJSON error: %v", err)
		}
		if string(data) != "[]" {
			t.Fatalf("MarshalJSON = %s, want []", data)
		}
	})

	t.Run("with_frames", func(t *testing.T) {
		frames := Frames{{File: "a.go", Line: 10, Function: "Foo"}}
		data, err := json.Marshal(frames)
		if err != nil {
			t.Fatalf("MarshalJSON error: %v", err)
		}
		var result []Frame
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}
		if len(result) != 1 || result[0].File != "a.go" {
			t.Fatalf("round-trip failed: %v", result)
		}
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
			if got := shortname(tt.input); got != tt.want {
				t.Fatalf("shortname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
