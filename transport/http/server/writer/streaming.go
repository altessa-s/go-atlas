// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"io"
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server/codec"
	"github.com/altessa-s/go-atlas/transport/internal/headers"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// WriteStream writes a response using streaming encoding (no buffering).
// Use for large responses that should not be buffered in memory.
//
// Note: Once headers are written, errors cannot change the HTTP status code.
// Any encoding errors will be returned but the client may receive partial response.
func (wr *Writer) WriteStream(w http.ResponseWriter, r *http.Request, data any) error {
	if w == nil {
		return ErrNilResponseWriter
	}
	if r == nil {
		return ErrNilRequest
	}

	structuredResponse, statusCode := wr.options.responseBuilder.Build(r, data)

	encoder, mimeType, err := wr.negotiateStreamingEncoder(r)
	if err != nil {
		return wr.writeError(w, r, err, http.StatusNotAcceptable)
	}

	// Write headers first (cannot be changed after body starts)
	w.Header().Set(headers.ContentType, mimeType+"; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(statusCode)

	// Stream directly to ResponseWriter
	if err := encoder.EncodeStream(w, structuredResponse); err != nil {
		// Headers already sent, cannot change response
		return coreerrs.WrapOperation(err, "stream response body")
	}

	return nil
}

// WriteStreamWithFlush writes a streaming response and flushes the buffer.
// Use for Server-Sent Events or scenarios requiring immediate client delivery.
func (wr *Writer) WriteStreamWithFlush(w http.ResponseWriter, r *http.Request, data any) error {
	if err := wr.WriteStream(w, r, data); err != nil {
		return err
	}

	// Flush if the ResponseWriter supports it
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// negotiateStreamingEncoder selects a streaming encoder based on Accept header.
func (wr *Writer) negotiateStreamingEncoder(r *http.Request) (codec.StreamingEncoder, string, error) {
	encoder, mimeType, err := wr.negotiateEncoder(r)
	if err != nil {
		return nil, "", err
	}

	// Check if encoder supports streaming natively
	if streamEncoder, ok := encoder.(codec.StreamingEncoder); ok {
		return streamEncoder, mimeType, nil
	}

	// Wrap regular encoder with streaming adapter
	return &streamingEncoderAdapter{encoder: encoder}, mimeType, nil
}

// streamingEncoderAdapter wraps a regular Encoder to implement StreamingEncoder.
// This provides a fallback for encoders that don't natively support streaming.
type streamingEncoderAdapter struct {
	encoder codec.Encoder
}

// EncodeStream implements codec.StreamingEncoder by encoding to bytes then writing.
// Note: This still buffers the entire response, use native streaming encoders for
// truly unbuffered streaming.
func (a *streamingEncoderAdapter) EncodeStream(w io.Writer, data any) error {
	bytes, err := a.encoder.Encode(data)
	if err != nil {
		return err
	}
	// CodeQL: False positive - 'bytes' is the output of encoder.Encode(data), which
	// produces properly encoded content (JSON/XML/Protobuf/etc). The encoder handles
	// all necessary escaping for the target format. This is not raw user input but
	// structured data that has been serialized through a codec.
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter
	_, err = w.Write(bytes) //nolint:gosec // Encoded structured data, not raw user input
	return err
}

// ContentType returns the content type from the wrapped encoder.
func (a *streamingEncoderAdapter) ContentType() string {
	return a.encoder.ContentType()
}
