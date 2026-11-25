// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/transport/http/server"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	tracingmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/tracing"
)

func TestPrefixedRoutesAndPrivateObservability(t *testing.T) {
	t.Parallel()

	const (
		pathSecret  = "/probe-secret-path"
		hostSecret  = "host-secret"
		agentSecret = "ua-secret"
		ipSecret    = "8.8.4.4"
	)
	for _, tc := range []struct {
		name    string
		handler func(io.Writer, *slog.HandlerOptions) slog.Handler
		optIn   bool
	}{
		{name: "text defaults", handler: func(w io.Writer, o *slog.HandlerOptions) slog.Handler { return slog.NewTextHandler(w, o) }},
		{name: "JSON defaults", handler: func(w io.Writer, o *slog.HandlerOptions) slog.Handler { return slog.NewJSONHandler(w, o) }},
		{name: "JSON opt-ins", handler: func(w io.Writer, o *slog.HandlerOptions) slog.Handler { return slog.NewJSONHandler(w, o) }, optIn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := httpconfig.Default()
			cfg.RouterPrefix = "/ops"
			mw := httpconfig.DefaultMiddlewares()
			mw.Logger.Enabled = true
			mw.Logger.StatusLevels = map[int]string{http.StatusNotFound: "error"}
			mw.Logger.LogClientDetails = tc.optIn
			mw.Metrics.Enabled = true
			mw.Tracing.Enabled = true
			mw.Tracing.RecordClientDetails = tc.optIn
			mw.Tracing.RecordRawErrors = tc.optIn
			mw.RequestId.Enabled = true
			mw.RealIp.Enabled = true
			mw.RealIp.LogClientDetails = tc.optIn
			mw.RealIp.LogRawErrors = tc.optIn
			mw.RealIp.TrustedProxies = []string{"127.0.0.1"}
			mw.Recovery.Enabled = true
			mw.Recovery.LogRawErrors = tc.optIn
			cfg.Middlewares = &mw

			var logs, spans bytes.Buffer
			logger := slog.New(tc.handler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			meter := metricstest.NewTestCollector()
			tracer := tracing.New(
				tracing.WithAdapter(console.New(console.WithWriter(&spans), console.WithPrettyPrint(false))),
				tracing.WithSampler(sampler.AlwaysOn()),
			)
			t.Cleanup(func() { _ = tracer.Shutdown(t.Context()) })
			base := start(t, factory.New(&cfg).WithInternalPrefix("").
				UseLogger(logger).UseTracer(tracer).UseMeter(meter).WithMiddlewares())

			for _, request := range []struct {
				path, route string
				status      int
			}{
				{path: "/ops/readyz", route: "/ops/readyz", status: http.StatusOK},
				{path: "/ops/unknown", route: "/_unknown", status: http.StatusNotFound},
				{path: pathSecret, route: "/_unknown", status: http.StatusNotFound},
			} {
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+request.path, nil)
				require.NoError(t, err)
				req.Host = hostSecret
				req.Header.Set("User-Agent", agentSecret)
				req.Header.Set("X-Forwarded-For", ipSecret)
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				_, err = io.Copy(io.Discard, resp.Body)
				require.NoError(t, resp.Body.Close())
				require.NoError(t, err)
				require.Equal(t, request.status, resp.StatusCode)
				require.NotEmpty(t, resp.Header.Get("X-Request-ID"))

				wantCount := float64(1)
				if request.path == pathSecret {
					wantCount = 2
				}
				require.Equal(t, wantCount, metricstest.GetCounterValue(t, meter,
					"test_http_server_requests_total", "method", "GET", "status", strconv.Itoa(request.status), "route", request.route))
			}
			resp, _ := get(t, base+"/ops/boom", nil)
			require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			require.NoError(t, tracer.ForceFlush(t.Context()))

			logOutput, spanOutput := logs.String(), spans.String()
			for _, secret := range []string{pathSecret, hostSecret, agentSecret, ipSecret, "kaboom"} {
				if tc.optIn {
					require.Contains(t, logOutput, secret)
					require.Contains(t, spanOutput, secret)
				} else {
					require.NotContains(t, logOutput, secret)
					require.NotContains(t, spanOutput, secret)
				}
			}
			require.Contains(t, logOutput, "panic recovered")
			// Match the completion log, not the metrics middleware's debug record.
			completions := map[string]int{}
			for line := range strings.Lines(logOutput) {
				if strings.Contains(line, "request completed") {
					completions["all"]++
					if strings.Contains(line, "ERROR") && strings.Contains(line, "/_unknown") {
						completions["missing"]++
					}
					if strings.Contains(line, "/ops/readyz") {
						completions["ready"]++
					}
				}
			}
			require.Equal(t, map[string]int{"all": 3, "missing": 2, "ready": 1}, completions)
			routeCounts := map[string]int{}
			decoder := json.NewDecoder(strings.NewReader(spanOutput))
			for decoder.More() {
				var span struct {
					Name       string         `json:"name"`
					Attributes map[string]any `json:"attributes"`
				}
				require.NoError(t, decoder.Decode(&span))
				route, ok := span.Attributes[tracingmw.HTTPRouteKey].(string)
				require.True(t, ok)
				require.Equal(t, "GET "+route, span.Name)
				routeCounts[route]++
			}
			require.Equal(t, map[string]int{"/ops/readyz": 1, "/_unknown": 2, "/ops/boom": 1}, routeCounts)
		})
	}
}
