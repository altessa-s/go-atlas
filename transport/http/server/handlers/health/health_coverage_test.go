// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/handlers/health"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"

	obsmhealth "github.com/altessa-s/go-atlas/observability/health"
)

var _ health.HealthReporter = (*obsmhealth.Coordinator)(nil)

type fakeReporter struct {
	status   obsmhealth.ServingStatus
	statuses map[string]obsmhealth.ServingStatus
	err      error
}

func (f fakeReporter) CheckStatus(context.Context, string) obsmhealth.ServingStatus {
	return f.status
}

func (f fakeReporter) ListStatuses(context.Context) (map[string]obsmhealth.ServingStatus, error) {
	return f.statuses, f.err
}

func TestHealthReporterSeam(t *testing.T) {
	t.Parallel()
	reporter := fakeReporter{status: obsmhealth.StatusServing, statuses: map[string]obsmhealth.ServingStatus{"api": obsmhealth.StatusServing}}
	rec := serve(t, health.Healthz(reporter), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	listErr := errors.New("list unavailable")
	rec = serve(t, health.Detailed(fakeReporter{err: listErr}), httptest.NewRequest(http.MethodGet, "/healthz/details", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":{"message":"Internal Server Error"}}`, rec.Body.String())
}

func newCoordinator(t *testing.T, services map[string]obsmhealth.ServingStatus) *obsmhealth.Coordinator {
	t.Helper()
	coord := obsmhealth.New()
	t.Cleanup(coord.Close)
	for name, st := range services {
		coord.RegisterService(name, obsmhealth.Func(func(context.Context) obsmhealth.ServingStatus { return st }))
	}
	return coord
}

func serve(t *testing.T, h func(writer.ReadWriter), req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rw := writer.NewReadWriter(rec, req, writer.New())
	h(rw)
	rw.Release()
	return rec
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		services map[string]obsmhealth.ServingStatus
		target   string
		wantCode int
		wantBody string
	}{
		{name: "no services is serving", target: "/healthz", wantCode: http.StatusOK, wantBody: "SERVING"},
		{
			name:     "all serving",
			services: map[string]obsmhealth.ServingStatus{"db": obsmhealth.StatusServing},
			target:   "/healthz", wantCode: http.StatusOK, wantBody: "SERVING",
		},
		{
			name:     "one not serving",
			services: map[string]obsmhealth.ServingStatus{"db": obsmhealth.StatusServing, "mq": obsmhealth.StatusNotServing},
			target:   "/healthz", wantCode: http.StatusServiceUnavailable,
		},
		{
			name:     "specific healthy service",
			services: map[string]obsmhealth.ServingStatus{"db": obsmhealth.StatusServing, "mq": obsmhealth.StatusNotServing},
			target:   "/healthz?service=db", wantCode: http.StatusOK, wantBody: "SERVING",
		},
		{
			name:     "specific unhealthy service",
			services: map[string]obsmhealth.ServingStatus{"db": obsmhealth.StatusServing, "mq": obsmhealth.StatusNotServing},
			target:   "/healthz?service=mq", wantCode: http.StatusServiceUnavailable,
		},
		{
			name:     "unknown service",
			services: map[string]obsmhealth.ServingStatus{"db": obsmhealth.StatusServing},
			target:   "/healthz?service=nope", wantCode: http.StatusServiceUnavailable,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			coord := newCoordinator(t, tc.services)
			rec := serve(t, health.Healthz(coord), httptest.NewRequest(http.MethodGet, tc.target, nil))
			require.Equal(t, tc.wantCode, rec.Code)
			if tc.wantBody != "" {
				var resp struct {
					Data health.Response `json:"data"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				require.Equal(t, tc.wantBody, resp.Data.Status)
			}
		})
	}
}

func TestReadyz(t *testing.T) {
	t.Parallel()

	t.Run("serving", func(t *testing.T) {
		t.Parallel()
		coord := newCoordinator(t, map[string]obsmhealth.ServingStatus{"db": obsmhealth.StatusServing})
		rec := serve(t, health.Readyz(coord), httptest.NewRequest(http.MethodGet, "/readyz", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var resp struct {
			Data health.Response `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, "SERVING", resp.Data.Status)
	})

	t.Run("not serving", func(t *testing.T) {
		t.Parallel()
		coord := newCoordinator(t, map[string]obsmhealth.ServingStatus{"db": obsmhealth.StatusNotServing})
		rec := serve(t, health.Readyz(coord), httptest.NewRequest(http.MethodGet, "/readyz", nil))
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("shutdown override", func(t *testing.T) {
		t.Parallel()
		coord := newCoordinator(t, map[string]obsmhealth.ServingStatus{"db": obsmhealth.StatusServing})
		coord.SetOverallStatus(obsmhealth.StatusNotServing)
		rec := serve(t, health.Readyz(coord), httptest.NewRequest(http.MethodGet, "/readyz", nil))
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})
}

func TestDetailed(t *testing.T) {
	t.Parallel()

	type detailed struct {
		Data health.ServiceResponse `json:"data"`
	}

	t.Run("all serving", func(t *testing.T) {
		t.Parallel()
		coord := newCoordinator(t, map[string]obsmhealth.ServingStatus{
			"db": obsmhealth.StatusServing,
			"mq": obsmhealth.StatusServing,
		})
		rec := serve(t, health.Detailed(coord), httptest.NewRequest(http.MethodGet, "/healthz/details", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var resp detailed
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, "SERVING", resp.Data.Status)
		require.Equal(t, map[string]health.ServiceStatus{
			"db": {Status: "SERVING"},
			"mq": {Status: "SERVING"},
		}, resp.Data.Services)
	})

	t.Run("one degraded", func(t *testing.T) {
		t.Parallel()
		coord := newCoordinator(t, map[string]obsmhealth.ServingStatus{
			"db": obsmhealth.StatusServing,
			"mq": obsmhealth.StatusDegraded,
		})
		rec := serve(t, health.Detailed(coord), httptest.NewRequest(http.MethodGet, "/healthz/details", nil))
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		var resp detailed
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, "NOT_SERVING", resp.Data.Status)
		require.Equal(t, "DEGRADED", resp.Data.Services["mq"].Status)
		require.Equal(t, "SERVING", resp.Data.Services["db"].Status)
	})

	t.Run("no services", func(t *testing.T) {
		t.Parallel()
		coord := newCoordinator(t, nil)
		rec := serve(t, health.Detailed(coord), httptest.NewRequest(http.MethodGet, "/healthz/details", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var resp detailed
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, "SERVING", resp.Data.Status)
		require.Empty(t, resp.Data.Services)
	})
}
