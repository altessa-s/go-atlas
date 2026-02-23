// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of HTTP servers
// and middleware.
//
// [Factory] reads structured config objects (from the config package) and
// produces fully wired server and middleware instances. All created
// components inherit the factory's logger, tracer, and TLS providers.
//
// Methods that need external dependencies (tracer, limiter, idempotency
// storage) return an error when the dependency is nil.
//
// [Factory.CreateMiddlewaresFromConfig] creates all enabled middleware and
// returns them in dependency-sorted order with duplicates removed.
//
// # Server Creation
//
//   - [Factory.CreateServerFromConfig] - HTTP server with TLS support
//
// # Middleware Creation
//
// Each Create*MiddlewareFromConfig method returns nil when the config is
// nil or disabled, allowing callers to collect results without nil checks:
//
//   - Body limit, CORS, idempotency, rate limiter, logger, Prometheus,
//     real IP, recovery, request ID, security headers, tracing
package factory
