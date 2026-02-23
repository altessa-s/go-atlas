// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package recovery provides middleware that recovers from panics in HTTP
// handlers, logs the panic with a stack trace, and returns a 500 response.
//
// [http.ErrAbortHandler] panics are re-raised rather than recovered, matching
// the standard library convention for deliberately aborting a handler.
//
// The middleware optionally logs the goroutine stack trace (enabled by
// default) and supports a custom [PanicHandler] for application-specific
// error responses. It declares a dependency on "requestid" for log
// correlation.
//
// # Example
//
//	mw := recovery.New(logger, recovery.WithLogStack())
//	handler := mw.Handler(yourHandler)
package recovery
