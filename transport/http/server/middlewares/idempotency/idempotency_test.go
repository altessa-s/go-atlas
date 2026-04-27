// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultKeyValidator(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid_uuid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"invalid", "not-a-uuid", true},
		{"empty", "", true},
		{"uppercase", "550E8400-E29B-41D4-A716-446655440000", true},
		{"mixed_case", "550e8400-E29B-41d4-a716-446655440000", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DefaultKeyValidator(tt.key)
			require.Equal(t, tt.wantErr, (err != nil))
		})
	}
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := &middleware{}
	require.Nil(t, m.Dependencies())
}

func TestBuildKey(t *testing.T) {
	m := &middleware{}
	key := m.buildKey("POST", "/api/v1/users", "550e8400-e29b-41d4-a716-446655440000")
	want := "idk:POST:/api/v1/users:550e8400-e29b-41d4-a716-446655440000"
	require.Equal(t, want, key)
}

func TestBuildKey_LeadingSlash(t *testing.T) {
	m := &middleware{}
	key := m.buildKey("GET", "/test", "abc")
	want := "idk:GET:/test:abc"
	require.Equal(t, want, key)
}

func TestIsSafeMethod(t *testing.T) {
	safe := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
	for _, method := range safe {
		require.True(t, isSafeMethod(method))
	}

	unsafe := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range unsafe {
		require.False(t, isSafeMethod(method), "isSafeMethod(%q) = true, want false", method)
	}
}

func TestCheckIdempotency_SkipsSafeMethod(t *testing.T) {
	m := New(nil, WithEnforceMandatory(true))

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			r := httptest.NewRequest(method, "/api/v1/users", nil)
			w := httptest.NewRecorder()

			key, _, err := m.checkIdempotency(w, r)
			require.NoError(t, err)
			require.Equal(t, "", key)
			require.Equal(t, http.StatusOK, w.Code)
		})
	}
}
