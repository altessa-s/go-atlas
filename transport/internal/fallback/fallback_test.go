// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fallback

import "testing"

func TestBehavior_String(t *testing.T) {
	tests := []struct {
		b    Behavior
		want string
	}{
		{Allow, "allow"},
		{Deny, "deny"},
		{Error, "error"},
		{Behavior("custom"), "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.b.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBehavior_ShouldAllow(t *testing.T) {
	tests := []struct {
		b    Behavior
		want bool
	}{
		{Allow, true},
		{Deny, false},
		{Error, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.b), func(t *testing.T) {
			if got := tt.b.ShouldAllow(); got != tt.want {
				t.Fatalf("ShouldAllow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBehavior_ShouldDeny(t *testing.T) {
	tests := []struct {
		b    Behavior
		want bool
	}{
		{Allow, false},
		{Deny, true},
		{Error, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.b), func(t *testing.T) {
			if got := tt.b.ShouldDeny(); got != tt.want {
				t.Fatalf("ShouldDeny() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBehavior_ShouldReturnError(t *testing.T) {
	tests := []struct {
		b    Behavior
		want bool
	}{
		{Allow, false},
		{Deny, false},
		{Error, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.b), func(t *testing.T) {
			if got := tt.b.ShouldReturnError(); got != tt.want {
				t.Fatalf("ShouldReturnError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBehavior_IsValid(t *testing.T) {
	tests := []struct {
		b    Behavior
		want bool
	}{
		{Allow, true},
		{Deny, true},
		{Error, true},
		{Behavior("unknown"), false},
		{Behavior(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.b), func(t *testing.T) {
			if got := tt.b.IsValid(); got != tt.want {
				t.Fatalf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBehavior_MustValidate_Valid(t *testing.T) {
	for _, b := range []Behavior{Allow, Deny, Error} {
		t.Run(string(b), func(t *testing.T) {
			// Should not panic
			b.MustValidate()
		})
	}
}

func TestBehavior_MustValidate_Invalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid behavior")
		}
	}()
	Behavior("bad").MustValidate()
}
