// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/logger"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/tracing"

	obstracing "github.com/altessa-s/go-atlas/observability/tracing"
)

func TestAuthCallbackRedaction(t *testing.T) {
	t.Parallel()
	const bearer = "credential-http-d8e2"
	const secret = "SECRET"
	callbackErr := fmt.Errorf("Bearer %s https://id.test/?api_key=%s: %w", bearer, secret, auth.ErrInvalidToken)
	for _, stage := range []string{"extract", "authenticate"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			var logs, spans bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			tr := obstracing.New(obstracing.WithAdapter(console.New(console.WithWriter(&spans), console.WithPrettyPrint(false))))
			defer func() { require.NoError(t, tr.Shutdown(t.Context())) }()
			extract := auth.TokenExtractorFunc(func(r *http.Request) (string, error) {
				if stage == "extract" {
					return "", callbackErr
				}
				return auth.ExtractBearerToken().ExtractToken(r)
			})
			authenticate := auth.AuthenticateFunc(func(context.Context, string) (any, error) { return nil, callbackErr })
			handleError := auth.ErrorHandlerFunc(func(w http.ResponseWriter, _ *http.Request, err error) {
				require.ErrorIs(t, err, callbackErr)
				require.ErrorIs(t, err, auth.ErrInvalidToken)
				http.Error(w, err.Error(), http.StatusUnauthorized)
			})
			authn := auth.New(auth.WithLogger(log), auth.WithTokenExtractor(extract), auth.WithAuthFunc(authenticate), auth.WithErrorHandler(handleError))
			chain := middlewares.NewChain(tracing.New(tr), logger.New(logger.Slog(log)), authn)
			h := chain.ThenFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unauthenticated handler called") })
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/private", nil)
			req.Header.Set("Authorization", "Bearer "+bearer)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusUnauthorized, rec.Code)
			require.NoError(t, tr.ForceFlush(t.Context()))
			require.Contains(t, logs.String(), "failed")
			require.Contains(t, logs.String(), "request completed")
			require.Contains(t, spans.String(), "http.status_code")
			for _, surface := range []string{rec.Body.String(), logs.String(), spans.String()} {
				require.NotContains(t, surface, bearer)
				require.NotContains(t, surface, secret)
			}
			require.Equal(t, "invalid_token\n", rec.Body.String())
		})
	}
}

type providerStatus struct{ code int }

func (e providerStatus) Error() string   { return "provider response with private details" }
func (e providerStatus) HTTPStatus() int { return e.code }

func TestAuthCallbackHTTPStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		code, want int
	}{
		{"forbidden", http.StatusForbidden, http.StatusForbidden},
		{"bad gateway", http.StatusBadGateway, http.StatusBadGateway},
		{"invalid success", http.StatusOK, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := auth.New(auth.WithTokenExtractor(auth.TokenExtractorFunc(func(*http.Request) (string, error) {
				return "", fmt.Errorf("extract: %w", providerStatus{tc.code})
			}))).Handler(nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
			require.Equal(t, tc.want, rec.Code)
			require.NotContains(t, rec.Body.String(), "private details")
		})
	}
}
