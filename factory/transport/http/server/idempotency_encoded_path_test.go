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
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	idempotencydata "github.com/altessa-s/go-atlas/reliability/idempotency"
)

// The mandatory pattern ^/orders/[^/]+$ is matched on the decoded path, the
// route /orders/{id} by net/http.ServeMux on the escaped one:
// "/orders/42%2Fx" decoded to three segments, missed the pattern and reached
// the handler without a key.
func TestIdempotencyClassifier_EncodedSlashRequiresKey(t *testing.T) {
	t.Parallel()

	c := httpconfig.DefaultIdempotencyMiddleware()
	c.Enabled = true
	c.MandatoryPatterns = []string{`^/orders/[^/]+$`}
	c.IgnorePaths = []string{"/v1/healthz"}
	b := New(&httpconfig.Config{Middlewares: &httpconfig.Middlewares{Idempotency: &c}}).
		UseIdempotency(idempotencydata.New(idempotencytest.NewMockIdempotencyStorage())).
		WithIdempotencyMiddleware()
	require.Empty(t, b.errs)
	require.Len(t, b.configMW, 1)

	tests := []struct {
		target     string
		wantServed bool
	}{
		{"/other/42", true},
		{"/other/caf%C3%A9", true},
		{"/orders/42", false},
		{"/orders/42%2Fx", false},
		{"/orders/42%2fx", false},
		{"/orders/%2E%2E", false},
	}
	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()

			served := false
			rt := std.New()
			rt.Use(b.configMW[0].Handler)
			for _, pattern := range []string{"/orders/{id}", "/other/{id}"} {
				rt.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
					served = true
					w.WriteHeader(http.StatusCreated)
				})
			}

			rec := httptest.NewRecorder()
			rt.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, tc.target, http.NoBody))

			require.Equal(t, tc.wantServed, served)
			if !tc.wantServed {
				require.Equal(t, http.StatusBadRequest, rec.Code, "a missing mandatory key")
			}
		})
	}
}
