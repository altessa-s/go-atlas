// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package observe implements a [httpclient.Observer] that records the HTTP
// client's events as metrics and derives health checks from them. It keeps
// the client package free of the observability stack: the client only emits
// events, this package turns them into [metrics.Meter] instruments and
// [health.Reporter] services.
//
// Create one [Observer] per client with [New] and install it with
// [httpclient.WithObserver].
//
// # Metrics
//
// All metrics are registered under the [WithMetricsSubsystem] subsystem
// (default [DefaultMetricsSubsystem], "http_client"):
//
//   - requests_total{method,status_class}: completed requests.
//   - request_errors_total{method}: requests that failed after all retries.
//   - retries_total: retry attempts.
//   - request_duration_seconds{method}: request duration, retries included.
//   - circuit_breaker_trips_total: attempts rejected by a circuit breaker.
//   - circuit_breaker_state{host}: breaker state (0=closed, 1=half-open,
//     2=open), labeled with the host of a per-host breaker and the name of
//     the client-wide one.
//
// # Health
//
// With a non-nil reporter the observer registers an aggregate checker
// (service [WithHealthServiceName], default [DefaultHealthServiceName]) when
// it is attached to a client. Its status is derived from the circuit
// breakers in service and a sliding window of retry rate
// ([WithHealthRetryWindow], [WithHealthRetryBuckets],
// [WithHealthRetryThreshold], [WithHealthRetryMinSamples]):
//
//   - all breakers closed and retry rate ≤ threshold → SERVING
//   - any half-open or open-while-others-closed, or retry rate above threshold → DEGRADED
//   - all breakers open → NOT_SERVING
//
// Subscribers receive an immediate update on every breaker state
// transition. [WithPerHostHealthChecks] also registers a "<service>.<host>"
// checker for every host with explicit circuit breaker settings.
//
// # Usage
//
//	coord := health.New()
//	defer coord.Close()
//
//	c := client.New(
//	    client.WithObserver(observe.New(meter, coord, observe.WithMetricsSubsystem("payments"))),
//	)
package observe
