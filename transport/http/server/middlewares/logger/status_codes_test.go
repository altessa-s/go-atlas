// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
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
		got := InternedStatusString(tt.code)
		require.Equal(t, tt.want, got)
	}
}

func TestInternedStatusString_SamePointer(t *testing.T) {
	// Interned strings should return the same string instance
	a := InternedStatusString(200)
	b := InternedStatusString(200)
	require.Equal(t, b, a)
}

func TestDefaultLogStatusCodes(t *testing.T) {
	require.NotEmpty(t, DefaultLogStatusCodes)
}

func TestDefaultLogStatusCodesSet(t *testing.T) {
	_, ok := DefaultLogStatusCodesSet[http.StatusOK]
	require.True(t, ok, "should contain 200")
	_, ok = DefaultLogStatusCodesSet[999]
	require.False(t, ok, "should not contain 999")
}

func BenchmarkInternedStatusString(b *testing.B) {
	for b.Loop() {
		InternedStatusString(http.StatusOK)
	}
}
