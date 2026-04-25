// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server/writer"

	obsmhealth "github.com/altessa-s/go-atlas/observability/health"
)

// Response is the response shape for the simple health endpoints
// ([K8sHealtz], [K8sReadyz], [Healthz], [Readyz]).
type Response struct {
	Status string `json:"status"`
}

// ServiceResponse is the response shape for [Detailed], a per-service
// health snapshot.
type ServiceResponse struct {
	Status   string                   `json:"status"`
	Services map[string]ServiceStatus `json:"services,omitempty"`
}

// ServiceStatus represents the health status of an individual service in
// a [ServiceResponse].
type ServiceStatus struct {
	Status string `json:"status"`
}

// K8sHealtz is a handler for Kubernetes liveness probes.
// It responds with a JSON [Response] containing status "ok".
func K8sHealtz(rw writer.ReadWriter) {
	_ = rw.Write(Response{Status: "ok"}) //nolint:errcheck // Best effort
}

// K8sReadyz is a handler for Kubernetes readiness probes.
// It responds with a JSON [Response] containing status "ok".
func K8sReadyz(rw writer.ReadWriter) {
	_ = rw.Write(Response{Status: "ok"}) //nolint:errcheck // Best effort
}

// Healthz returns a handler that checks overall health via the
// [obsmhealth.Coordinator]. It returns HTTP 200 when all services are
// healthy, HTTP 503 otherwise. The optional service query parameter
// checks a specific service.
//
// Example:
//
//	srv.Handle("/healthz", health.Healthz(coordinator))
func Healthz(coord *obsmhealth.Coordinator) func(rw writer.ReadWriter) {
	return func(rw writer.ReadWriter) {
		ctx := rw.Request().Context()
		service := rw.Request().URL.Query().Get("service")

		status := coord.CheckStatus(ctx, service)

		resp := Response{Status: status.String()}

		if status != obsmhealth.StatusServing {
			_ = rw.WriteError(nil, http.StatusServiceUnavailable) //nolint:errcheck
			return
		}

		_ = rw.Write(resp) //nolint:errcheck // Best effort
	}
}

// Readyz returns a handler that checks readiness via the
// [obsmhealth.Coordinator]. It returns HTTP 200 when overall health is
// SERVING, HTTP 503 otherwise.
//
// Example:
//
//	srv.Handle("/readyz", health.Readyz(coordinator))
func Readyz(coord *obsmhealth.Coordinator) func(rw writer.ReadWriter) {
	return func(rw writer.ReadWriter) {
		ctx := rw.Request().Context()

		status := coord.CheckStatus(ctx, "")

		resp := Response{Status: status.String()}

		if status != obsmhealth.StatusServing {
			_ = rw.WriteError(nil, http.StatusServiceUnavailable) //nolint:errcheck
			return
		}

		_ = rw.Write(resp) //nolint:errcheck // Best effort
	}
}

// Detailed returns a handler that lists all registered services and their
// health statuses. It returns HTTP 200 when all services are healthy,
// HTTP 503 if any service is unhealthy.
//
// Example:
//
//	srv.Handle("/healthz/details", health.Detailed(coordinator))
func Detailed(coord *obsmhealth.Coordinator) func(rw writer.ReadWriter) {
	return func(rw writer.ReadWriter) {
		ctx := rw.Request().Context()

		statuses, err := coord.ListStatuses(ctx)
		if err != nil {
			_ = rw.WriteError(err, http.StatusInternalServerError) //nolint:errcheck
			return
		}

		overall := obsmhealth.StatusServing
		services := make(map[string]ServiceStatus, len(statuses))
		for name, s := range statuses {
			services[name] = ServiceStatus{Status: s.String()}
			if s != obsmhealth.StatusServing {
				overall = obsmhealth.StatusNotServing
			}
		}

		resp := ServiceResponse{
			Status:   overall.String(),
			Services: services,
		}

		if overall != obsmhealth.StatusServing {
			rw.ResponseWriter().WriteHeader(http.StatusServiceUnavailable)
		}

		_ = rw.Write(resp) //nolint:errcheck // Best effort
	}
}
