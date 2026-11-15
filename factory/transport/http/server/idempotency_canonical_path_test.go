// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/idempotency/idempotencytest"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	idempotencydata "github.com/altessa-s/go-atlas/reliability/idempotency"
)

// Another spelling of a mandatory path used to match no mandatory entry and
// fall back to enforceMandatory=false, so the request went through without a
// key. A path that is not canonical now requires one.
func TestIdempotencyClassifier_NonCanonicalPathRequiresKey(t *testing.T) {
	t.Parallel()

	c := httpconfig.DefaultIdempotencyMiddleware()
	c.Enabled = true
	c.MandatoryPaths = []string{"/payments"}
	c.MandatoryPatterns = []string{`^/orders/[^/]+$`}
	b := New(&httpconfig.Config{Middlewares: &httpconfig.Middlewares{Idempotency: &c}}).
		UseIdempotency(idempotencydata.New(idempotencytest.NewMockIdempotencyStorage())).
		WithIdempotencyMiddleware()
	require.Empty(t, b.errs)
	require.Len(t, b.configMW, 1)

	tests := []struct {
		path       string
		wantServed bool
	}{
		{"/other", true},
		{"/payments/", true}, // a different canonical path; the routers do not serve /payments for it
		{"/payments", false},
		{"/orders/42", false},
		{"//payments", false},
		{"/./payments", false},
		{"/x/../payments", false},
		{"/payments/.", false},
		{"/orders//42", false},
		{"/other/../orders/42", false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			served := false
			h := b.configMW[0].Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				served = true
				w.WriteHeader(http.StatusCreated)
			}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody)
			req.URL.Path = tc.path
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, tc.wantServed, served)
			if !tc.wantServed {
				require.Equal(t, http.StatusBadRequest, rec.Code, "a missing mandatory key")
			}
		})
	}
}
