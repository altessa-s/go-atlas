// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating gRPC servers
// and interceptors from configuration.
//
// [ServerBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ServerBuilder.Build] time.
//
//	srv, err := factory.New(cfg.Grpc).
//	    UseLogger(logger).
//	    UseTracer(tracer).
//	    WithInterceptors().
//	    Build()
//
// # Interceptor Control
//
// Two levels of interceptor control are available:
//
//   - [ServerBuilder.WithInterceptors] -- all config-based interceptors at once;
//     accepts optional exclude arguments (e.g., auth.ID, cache.ID) to skip specific interceptors.
//   - [ServerBuilder.WithLoggerInterceptor], [ServerBuilder.WithTracingInterceptor], etc. -- individual config-based interceptors
//
// Dependencies must be set via Use*() methods before calling With*Interceptor methods:
//
//	srv, err := factory.New(cfg.Grpc).
//	    UseLogger(logger).
//	    UseTlsProviders(providers).
//	    UseTracer(tracer).
//	    UseLimiter(limiter).
//	    UseAuth(authFn, clientAuth).
//	    UseHealthChecker(healthChecker).
//	    WithInterceptors().
//	    Build()
//
// To skip specific interceptors, pass their typed IDs:
//
//	srv, err := factory.New(cfg.Grpc).
//	    UseLogger(logger).
//	    WithInterceptors(auth.ID, cache.ID).
//	    Build()
package factory
