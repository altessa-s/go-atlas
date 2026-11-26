// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package respwriter holds the HTTP response writer wrapper shared by the
// middlewares, and the marker contract between the response writer package
// and the error interceptor.
//
// # Recorder
//
// [Recorder] wraps an [net/http.ResponseWriter] and records the final status,
// the number of body bytes and, optionally, a bounded prefix of the body.
// Its record always matches what the client receives: a final status
// (200 and above, or 101) is the first one forwarded, and a flush that
// reaches the underlying writer commits the implicit 200 the server sends
// then, so a later [net/http.ResponseWriter.WriteHeader] cannot change the
// record while the wire keeps the committed status. [Recorder] exposes
// Flush, FlushError, Hijack and Push on top of Unwrap, so
// [net/http.NewResponseController] and type assertions on the wrapped writer
// both work.
//
// # Structured responses
//
// A writer that already produced a complete structured error response calls
// [MarkStructured] before writing the header. Every wrapper on the
// Unwrap chain implementing [StructuredMarker] is told, and the error
// interceptor then forwards the response as is instead of rewriting it.
//
// # Example
//
//	rec := respwriter.NewRecorder(w, true, 1<<20)
//	defer rec.Release()
//	next.ServeHTTP(rec, r)
//	status := rec.StatusCode()
package respwriter
