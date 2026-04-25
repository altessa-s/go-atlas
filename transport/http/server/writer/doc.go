// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package writer provides HTTP response writing with content negotiation
// and request body decoding.
//
// [Writer] is the core type: it selects a [codec.Encoder] by negotiating
// the Accept header against the [codec.Registry], then encodes the response
// through a configurable [Builder] (default: [Default]) that wraps data in
// a [Response] envelope. Error responses use the [Coder], [Messager], and
// [HTTPStatuser] interfaces to extract structured error details from Go
// error values.
//
// [ReadWriter] combines read and write operations into a per-request helper
// that is pooled for efficiency. Handlers receive a ReadWriter from the
// server and should call Release when done.
//
// For large payloads, [Writer.WriteStream] and [Writer.WriteStreamWithFlush]
// write directly to the response without buffering the entire body.
//
// # Example
//
//	w := writer.New()
//	w.Write(rw, r, map[string]string{"status": "ok"})
package writer
