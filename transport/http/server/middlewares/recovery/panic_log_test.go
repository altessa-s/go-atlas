// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"

	sharedid "github.com/altessa-s/go-atlas/transport/requestid"
)

func TestRecoveryLogsResolvedRequestID(t *testing.T) {
	t.Parallel()
	const header = "X-Correlation-ID"
	for _, tc := range []struct{ name, incoming string }{
		{"incoming", "550e8400-e29b-41d4-a716-446655440000"}, {"generated", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var log bytes.Buffer
			id := requestid.New(sharedid.NewGenerator(sharedid.WithHeaderName(header)))
			guard := recovery.New(slog.New(slog.NewJSONHandler(&log, nil)))
			chain, err := middlewares.NewChain(id, guard).Ordered()
			require.NoError(t, err)
			var resolved string
			handler := chain.ThenFunc(func(w http.ResponseWriter, r *http.Request) {
				resolved = requestid.FromContext(r.Context())
				w.Header().Del(header)
				panic("boom")
			})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/boom", nil)
			req.Header.Set(header, tc.incoming)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			require.Equal(t, http.StatusInternalServerError, rec.Code)
			require.NotEmpty(t, resolved)
			if tc.incoming != "" {
				require.Equal(t, tc.incoming, resolved)
			}
			var entry struct {
				Message   string `json:"msg"`
				RequestID string `json:"request_id"`
			}
			require.NoError(t, json.Unmarshal(log.Bytes(), &entry))
			require.Equal(t, "panic recovered", entry.Message)
			require.Equal(t, resolved, entry.RequestID)
		})
	}
}
