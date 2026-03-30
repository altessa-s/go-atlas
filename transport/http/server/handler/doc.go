// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package handler provides standard HTTP handlers for operational endpoints.
//
// The health-check handlers ([Ping], [K8sHealtz], [K8sReadyz]) are
// [server.HandlerFunc] values that write JSON via [writer.ReadWriter].
// The debug and metrics helpers ([Pprof], [PrometheusMetrics]) accept a
// [router.Router] and mount a subrouter at the conventional path prefix.
//
// # Static Health Check Handlers
//
// These always return 200 OK — suitable for basic liveness probes when no
// [health.Coordinator] is available:
//
//	srv.Handle("/ping", handler.Ping)
//	srv.Handle("/healthz", handler.K8sHealtz)
//	srv.Handle("/readyz", handler.K8sReadyz)
//
// # Coordinator-backed Health Check Handlers
//
// When an [health.Coordinator] is available, use these handlers to check
// actual service health and return proper HTTP status codes (200/503):
//
//	srv.Handle("/healthz", handler.Healthz(coordinator))
//	srv.Handle("/readyz", handler.Readyz(coordinator))
//	srv.Handle("/healthz/details", handler.HealthzDetailed(coordinator))
//
// [Healthz] and [Readyz] return [HealthResponse] with the [health.ServingStatus]
// string. [HealthzDetailed] returns [ServiceHealthResponse] with per-service
// breakdown.
//
// # Debug and Metrics
//
//	handler.Pprof(router)              // mounts at /pprof
//	handler.PrometheusMetrics(router)  // mounts at /metrics
package handler
