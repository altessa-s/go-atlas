// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fallback

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
			got := tt.b.String()
			require.Equal(t, tt.want, got)
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
			got := tt.b.ShouldAllow()
			require.Equal(t, tt.want, got)
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
			got := tt.b.ShouldDeny()
			require.Equal(t, tt.want, got)
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
			got := tt.b.ShouldReturnError()
			require.Equal(t, tt.want, got)
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
			got := tt.b.IsValid()
			require.Equal(t, tt.want, got)
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
		r := recover()
		require.NotNil(t, r)
	}()
	Behavior("bad").MustValidate()
}
