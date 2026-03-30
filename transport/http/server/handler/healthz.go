// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package handler

import (
	"net/http"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

// HealthResponse is the response for health check endpoints.
type HealthResponse struct {
	Status string `json:"status"`
}

// ServiceHealthResponse is the response for per-service health check endpoints.
type ServiceHealthResponse struct {
	Status   string                   `json:"status"`
	Services map[string]ServiceStatus `json:"services,omitempty"`
}

// ServiceStatus represents an individual service's health.
type ServiceStatus struct {
	Status string `json:"status"`
}

// K8sHealtz is a handler for Kubernetes liveness probes.
// It responds with a JSON [HealthResponse] containing status "ok".
func K8sHealtz(rw writer.ReadWriter) {
	_ = rw.Write(HealthResponse{Status: "ok"}) //nolint:errcheck // Best effort
}

// K8sReadyz is a handler for Kubernetes readiness probes.
// It responds with a JSON [HealthResponse] containing status "ok".
func K8sReadyz(rw writer.ReadWriter) {
	_ = rw.Write(HealthResponse{Status: "ok"}) //nolint:errcheck // Best effort
}

// Healthz returns a handler that checks overall health via the [health.Coordinator].
// It returns HTTP 200 when all services are healthy, HTTP 503 otherwise.
// The optional service query parameter checks a specific service.
//
// Example:
//
//	srv.Handle("/healthz", handler.Healthz(coordinator))
func Healthz(coord *health.Coordinator) func(rw writer.ReadWriter) {
	return func(rw writer.ReadWriter) {
		ctx := rw.Request().Context()
		service := rw.Request().URL.Query().Get("service")

		status := coord.CheckStatus(ctx, service)

		resp := HealthResponse{Status: status.String()}

		if status != health.StatusServing {
			_ = rw.WriteError(nil, http.StatusServiceUnavailable) //nolint:errcheck
			return
		}

		_ = rw.Write(resp) //nolint:errcheck // Best effort
	}
}

// Readyz returns a handler that checks readiness via the [health.Coordinator].
// It returns HTTP 200 when overall health is SERVING, HTTP 503 otherwise.
//
// Example:
//
//	srv.Handle("/readyz", handler.Readyz(coordinator))
func Readyz(coord *health.Coordinator) func(rw writer.ReadWriter) {
	return func(rw writer.ReadWriter) {
		ctx := rw.Request().Context()

		status := coord.CheckStatus(ctx, "")

		resp := HealthResponse{Status: status.String()}

		if status != health.StatusServing {
			_ = rw.WriteError(nil, http.StatusServiceUnavailable) //nolint:errcheck
			return
		}

		_ = rw.Write(resp) //nolint:errcheck // Best effort
	}
}

// HealthzDetailed returns a handler that lists all registered services and their
// health statuses. It returns HTTP 200 when all services are healthy, HTTP 503
// if any service is unhealthy.
//
// Example:
//
//	srv.Handle("/healthz/details", handler.HealthzDetailed(coordinator))
func HealthzDetailed(coord *health.Coordinator) func(rw writer.ReadWriter) {
	return func(rw writer.ReadWriter) {
		ctx := rw.Request().Context()

		statuses, err := coord.ListStatuses(ctx)
		if err != nil {
			_ = rw.WriteError(err, http.StatusInternalServerError) //nolint:errcheck
			return
		}

		overall := health.StatusServing
		services := make(map[string]ServiceStatus, len(statuses))
		for name, s := range statuses {
			services[name] = ServiceStatus{Status: s.String()}
			if s != health.StatusServing {
				overall = health.StatusNotServing
			}
		}

		resp := ServiceHealthResponse{
			Status:   overall.String(),
			Services: services,
		}

		if overall != health.StatusServing {
			rw.ResponseWriter().WriteHeader(http.StatusServiceUnavailable)
		}

		_ = rw.Write(resp) //nolint:errcheck // Best effort
	}
}
