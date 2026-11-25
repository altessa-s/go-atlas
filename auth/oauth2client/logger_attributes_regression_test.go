// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/oauth2"
)

type errorAttributeHandler struct {
	slog.Handler
	records []slog.Record
}

func (h *errorAttributeHandler) Handle(_ context.Context, record slog.Record) error {
	h.records = append(h.records, record.Clone())
	return nil
}

func TestLoggerAttributesWithholdResponseErrors(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"token exchange", "jwt bearer"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			handler := &errorAttributeHandler{Handler: slog.NewTextHandler(io.Discard, nil)}
			client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable, Request: req,
					Header: http.Header{"Content-Type": []string{"application/json"}},
					Body:   io.NopCloser(strings.NewReader(`{"error":"server_error","error_description":"private-response-secret"}`)),
				}, nil
			})}
			var err error
			if name == "token exchange" {
				_, err = mustExchanger(t, "https://idp.example/token", "svc", clientSecret,
					oauth2client.WithExchangerHTTPClient(client),
					oauth2client.WithExchangerLogger(slog.New(handler)),
					oauth2client.WithExchangerRetryAttempts(1),
					oauth2client.WithExchangerRetryBaseDelay(time.Millisecond),
					oauth2client.WithExchangerRetryMaxDelay(time.Millisecond)).
					Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: bearerToken})
			} else {
				key, _ := rsaKey(t, "key")
				flow, createErr := oauth2client.NewJWTBearer("https://idp.example/token", "svc", key,
					oauth2client.WithJWTBearerHTTPClient(client),
					oauth2client.WithJWTBearerLogger(slog.New(handler)),
					oauth2client.WithJWTBearerRetryAttempts(1),
					oauth2client.WithJWTBearerRetryBaseDelay(time.Millisecond),
					oauth2client.WithJWTBearerRetryMaxDelay(time.Millisecond))
				require.NoError(t, createErr)
				_, err = flow.Token(t.Context())
			}
			var retrieved *oauth2.RetrieveError
			require.ErrorAs(t, err, &retrieved)
			require.Len(t, handler.records, 2)
			var walk func(slog.Attr)
			walk = func(attr slog.Attr) {
				value := attr.Value.Resolve()
				if value.Kind() == slog.KindGroup {
					for _, child := range value.Group() {
						walk(child)
					}
					return
				}
				if loggedErr, ok := value.Any().(error); ok {
					require.False(t, errors.As(loggedErr, &retrieved), "response-bearing log attribute %q", attr.Key)
				}
				if attr.Key == "err" {
					require.Equal(t, slog.KindString, value.Kind())
					require.NotContains(t, value.String(), "private-response-secret")
				}
			}
			for _, record := range handler.records {
				record.Attrs(func(attr slog.Attr) bool {
					walk(attr)
					return true
				})
			}
		})
	}
}
