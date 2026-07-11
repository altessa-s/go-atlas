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
// # Response sanitization
//
// When the response value is a proto.Message, [Writer.Write] and
// [Writer.WriteStream] strip fields annotated google.api.field_behavior =
// INPUT_ONLY before encoding, so write-path secrets (passwords, one-time
// tokens) never leak back to clients on the read path. Sanitization is on by
// default; the caller's message is never mutated (the fields are cleared on a
// clone, and only when a populated INPUT_ONLY field is actually present).
// Non-proto responses are unaffected. Disable it with
// [WithResponseSanitizationDisabled].
//
// # Example
//
//	w := writer.New()
//	w.Write(rw, r, map[string]string{"status": "ok"})
package writer
