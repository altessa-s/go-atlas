// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the otlp block of the tracing configuration to an
// OTLP tracing adapter for factory/observability/tracing, building the
// exporter's client: an SSRF-guarded HTTP client pinned to the collector for
// OTLP/HTTP, a resilient gRPC client for OTLP/gRPC, both with the configured
// proxy policy.
//
// The tracing factory builds only the console and noop adapters; register
// this one for the otlp type:
//
//	tracer, err := tracingfactory.New(&cfg.Tracing).
//		UseAdapter(observabilityconfig.TracingTypeOTLP, otlpfactory.Builder()).
//		Build(ctx)
package factory
