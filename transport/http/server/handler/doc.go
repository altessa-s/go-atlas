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
// # Health Check Handlers
//
//	srv.Handle("/ping", handler.Ping)
//	srv.Handle("/healthz", handler.K8sHealtz)
//	srv.Handle("/readyz", handler.K8sReadyz)
//
// # Debug and Metrics
//
//	handler.Pprof(router)              // mounts at /pprof
//	handler.PrometheusMetrics(router)  // mounts at /metrics
package handler
