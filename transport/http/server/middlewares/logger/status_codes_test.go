// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"net/http"
	"testing"
)

func TestInternedStatusString(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{http.StatusOK, "200"},
		{http.StatusNotFound, "404"},
		{http.StatusInternalServerError, "500"},
		{999, "999"},
	}
	for _, tt := range tests {
		if got := InternedStatusString(tt.code); got != tt.want {
			t.Fatalf("InternedStatusString(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestInternedStatusString_SamePointer(t *testing.T) {
	// Interned strings should return the same string instance
	a := InternedStatusString(200)
	b := InternedStatusString(200)
	if a != b {
		t.Fatal("interned strings should be identical")
	}
}

func TestDefaultLogStatusCodes(t *testing.T) {
	if len(DefaultLogStatusCodes) == 0 {
		t.Fatal("DefaultLogStatusCodes should not be empty")
	}
}

func TestDefaultLogStatusCodesSet(t *testing.T) {
	if _, ok := DefaultLogStatusCodesSet[http.StatusOK]; !ok {
		t.Fatal("should contain 200")
	}
	if _, ok := DefaultLogStatusCodesSet[999]; ok {
		t.Fatal("should not contain 999")
	}
}

func TestFieldKeys(t *testing.T) {
	if string(FieldKeyHTTPMethod) != "http.method" {
		t.Fatal("wrong field key")
	}
	if string(FieldKeyHTTPStatus) != "http.status" {
		t.Fatal("wrong field key")
	}
}

func BenchmarkInternedStatusString(b *testing.B) {
	for b.Loop() {
		InternedStatusString(http.StatusOK)
	}
}
