// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
			if (err != nil) != tt.wantErr {
				t.Fatalf("DefaultKeyValidator(%q) err = %v, wantErr = %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := &middleware{}
	if m.Dependencies() != nil {
		t.Fatal("Dependencies should be nil")
	}
}

func TestBuildKey(t *testing.T) {
	m := &middleware{}
	key := m.buildKey("POST", "/api/v1/users", "550e8400-e29b-41d4-a716-446655440000")
	want := "idk:POST:/api/v1/users:550e8400-e29b-41d4-a716-446655440000"
	if key != want {
		t.Fatalf("buildKey = %q, want %q", key, want)
	}
}

func TestBuildKey_LeadingSlash(t *testing.T) {
	m := &middleware{}
	key := m.buildKey("GET", "/test", "abc")
	want := "idk:GET:/test:abc"
	if key != want {
		t.Fatalf("buildKey = %q, want %q", key, want)
	}
}

func TestIsSafeMethod(t *testing.T) {
	safe := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
	for _, method := range safe {
		if !isSafeMethod(method) {
			t.Fatalf("isSafeMethod(%q) = false, want true", method)
		}
	}

	unsafe := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range unsafe {
		if isSafeMethod(method) {
			t.Fatalf("isSafeMethod(%q) = true, want false", method)
		}
	}
}

func TestCheckIdempotency_SkipsSafeMethod(t *testing.T) {
	m := New(nil, WithEnforceMandatory(true))

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			r := httptest.NewRequest(method, "/api/v1/users", nil)
			w := httptest.NewRecorder()

			key, err := m.checkIdempotency(w, r)
			if err != nil {
				t.Fatalf("checkIdempotency returned error for %s: %v", method, err)
			}
			if key != "" {
				t.Fatalf("checkIdempotency returned key %q for %s, want empty", key, method)
			}
			if w.Code != http.StatusOK {
				t.Fatalf("response status = %d, want %d", w.Code, http.StatusOK)
			}
		})
	}
}
