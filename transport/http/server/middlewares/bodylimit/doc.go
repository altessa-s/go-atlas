// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package bodylimit provides middleware that rejects requests whose body
// exceeds a configured size limit.
//
// The middleware performs two checks: a fast pre-flight Content-Length header
// check (returning 413 immediately), and a streaming enforcement via
// [http.MaxBytesReader] to catch chunked or misreported bodies.
//
// Error responses use the [responder.WriteError] helper for content-negotiated
// structured responses (JSON/XML based on Accept header).
//
// # Example
//
//	srv.Use(bodylimit.Middleware(10 * 1024 * 1024)) // 10 MB limit
package bodylimit
