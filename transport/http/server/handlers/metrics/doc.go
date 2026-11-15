// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package metrics mounts a metrics exposition handler on a [router.Router].
// Pair it with the application's main HTTP server (or an internal admin
// port) to expose the standard `/metrics` endpoint. The handler comes from
// the metrics backend (promadapter.DefaultHandler() for the Prometheus
// default gatherer), so the package depends on no metrics library.
//
// # Usage
//
//	import metricsh "github.com/altessa-s/go-atlas/transport/http/server/handlers/metrics"
//
//	metricsh.Mount(internalRouter, promadapter.DefaultHandler()) // exposes /metrics on internalRouter
package metrics
