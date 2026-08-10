// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bodylimit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzLimitHoldsAgainstADishonestContentLength is the oracle for a cap whose
// whole job is to bound what an untrusted client can make the process read.
//
// Content-Length is a client claim, so a middleware that trusts it enforces
// nothing: an attacker declares 10 bytes and sends megabytes. The property is
// therefore stated over what the handler could actually read — never more than
// the cap — regardless of what the request declared.
func FuzzLimitHoldsAgainstADishonestContentLength(f *testing.F) {
	f.Add(int64(10), int64(0), 100)   // Declares nothing, sends 100.
	f.Add(int64(10), int64(5), 100)   // Declares 5, sends 100.
	f.Add(int64(100), int64(100), 50) // Honest and within.
	f.Add(int64(0), int64(0), 10)     // Cap disabled.
	f.Add(int64(1), int64(-1), 4096)  // Unknown length (chunked).

	f.Fuzz(func(t *testing.T, maxSize, declared int64, actual int) {
		if maxSize < 0 || actual < 0 || actual > 1<<20 {
			t.Skip("sizes outside the plausible range say nothing about the cap")
		}

		var read int64
		handler := Middleware(maxSize)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			n, _ := io.Copy(io.Discard, r.Body)
			read = n
		}))

		body := strings.Repeat("x", actual)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/fuzz", strings.NewReader(body))
		req.ContentLength = declared

		handler.ServeHTTP(httptest.NewRecorder(), req)

		if maxSize == 0 {
			return // A zero cap is documented as "no limit".
		}

		require.LessOrEqual(t, read, maxSize,
			"the handler read %d bytes past a %d-byte cap (declared %d, sent %d)",
			read, maxSize, declared, actual)
	})
}

// FuzzOversizeDeclarationIsRejectedBeforeTheHandler pins the cheap half of the
// defense: a request that admits to being too large never reaches the handler.
//
// The pre-check exists so an obvious violation costs nothing to refuse. A
// handler that runs anyway has already paid for whatever it does before
// touching the body — a database lookup, an authorization call — which is the
// work the cap was supposed to avoid.
func FuzzOversizeDeclarationIsRejectedBeforeTheHandler(f *testing.F) {
	f.Add(int64(10), int64(100))
	f.Add(int64(10), int64(10))
	f.Add(int64(1), int64(2))

	f.Fuzz(func(t *testing.T, maxSize, declared int64) {
		if maxSize <= 0 || declared < 0 || declared > 1<<30 {
			t.Skip("sizes outside the plausible range say nothing about the pre-check")
		}

		var reached bool
		handler := Middleware(maxSize)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached = true
		}))

		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/fuzz", strings.NewReader(""))
		req.ContentLength = declared

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if declared > maxSize {
			require.False(t, reached,
				"a request declaring %d bytes reached the handler behind a %d-byte cap", declared, maxSize)
			require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
			return
		}

		require.True(t, reached,
			"a request declaring %d bytes was refused by a %d-byte cap", declared, maxSize)
	})
}
