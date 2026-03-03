// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating HTTP servers
// and middleware from configuration.
//
// [ServerBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ServerBuilder.Build] time.
//
//	srv, err := factory.New(cfg.Http).
//	    UseLogger(logger).
//	    UseTracer(tracer).
//	    WithMiddlewares().
//	    Build()
//
// # Middleware Control
//
// Three levels of middleware control are available:
//
//   - [ServerBuilder.WithMiddlewares] — all config-based middleware at once
//   - [ServerBuilder.WithBodyLimitMiddleware], [ServerBuilder.WithCorsMiddleware], etc. — individual config-based middleware
//   - [ServerBuilder.WithMiddleware] — custom pre-built middleware instances
//
// Use Without* methods to disable specific middleware after [ServerBuilder.WithMiddlewares]:
//
//	srv, err := factory.New(cfg.Http).
//	    UseLogger(logger).
//	    WithMiddlewares().
//	    WithoutCorsMiddleware().
//	    Build()
package factory
