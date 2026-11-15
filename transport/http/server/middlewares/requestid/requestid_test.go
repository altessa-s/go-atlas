// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/requestid"
)

func TestRequestId_GeneratesIfMissing(t *testing.T) {
	gen := requestid.NewGenerator()
	mw := RequestId(gen)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := FromContext(r.Context())
		require.NotEqual(t, "", id)
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEqual(t, "", rec.Header().Get(gen.HeaderName()))
}

func TestRequestId_UsesExisting(t *testing.T) {
	gen := requestid.NewGenerator()
	mw := RequestId(gen)
	existingID := "550e8400-e29b-41d4-a716-446655440000"

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := FromContext(r.Context())
		require.Equal(t, existingID, id)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(gen.HeaderName(), existingID)
	handler.ServeHTTP(rec, req)
}

func TestRequestId_SkipsOptions(t *testing.T) {
	gen := requestid.NewGenerator()
	mw := RequestId(gen)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	handler.ServeHTTP(rec, req)

	require.True(t, called, "handler should be called for OPTIONS")
}

func TestFromContext_Empty(t *testing.T) {
	id := FromContext(t.Context())
	require.Equal(t, "", id)
}

func TestNewContext(t *testing.T) {
	ctx := NewContext(t.Context(), "test-id")
	require.Equal(t, "test-id", FromContext(ctx))
}

func TestRequestId_GenerationDisabled(t *testing.T) {
	t.Parallel()

	gen := requestid.NewGenerator(requestid.WithGenerateIfMissing(false))
	const validID = "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name     string
		header   string
		wantCode int
		wantID   string
	}{
		{"missing_id_is_rejected", "", http.StatusBadRequest, ""},
		{"invalid_id_is_rejected", "not-a-uuid", http.StatusBadRequest, ""},
		{"valid_id_passes", validID, http.StatusOK, validID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotID string
			handler := RequestId(gen)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				gotID = FromContext(r.Context())
			}))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tc.header != "" {
				req.Header.Set(gen.HeaderName(), tc.header)
			}
			handler.ServeHTTP(rec, req)

			require.Equal(t, tc.wantCode, rec.Code)
			require.Equal(t, tc.wantID, gotID)
		})
	}
}
