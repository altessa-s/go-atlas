// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package observability provides shared structured-logging field keys and
// utilities used by both HTTP middlewares and gRPC interceptors.
//
// All field key strings are interned at package init to minimize
// allocations in high-throughput request logging paths.
//
// # Logging level conventions
//
//   - Debug: routine operations (ignored paths, start/end, cache hits)
//   - Warn:  configuration warnings, authentication failures, non-critical errors
//   - Error: panics, storage failures, critical errors
//
// Example:
//
//	fields := observability.Fields{
//	    {Key: observability.FieldKeyClientPeerIP, Value: "192.168.1.1"},
//	}
//	ctx = observability.InjectFields(ctx, fields)
package observability
