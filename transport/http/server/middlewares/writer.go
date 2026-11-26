// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"bufio"
	"net"
	"net/http"
	"sync"

	"github.com/altessa-s/go-atlas/internal/transport/respwriter"
)

const (
	// MaxCaptureBodySize is the maximum size of response body to capture (1MB).
	// Bodies larger than this will be truncated.
	MaxCaptureBodySize = 1024 * 1024
)

// ResponseWriter wraps [http.ResponseWriter] to capture the status code and
// optionally the response body. Instances are obtained from a [sync.Pool];
// create them with [NewResponseWriter] and return them with
// [ResponseWriter.Release] when the request is complete.
//
// The recorded status always matches what the client receives: a flush
// commits the implicit 200 and a later WriteHeader cannot change it.
// ResponseWriter forwards Flush, FlushError, Hijack and Push to the wrapped
// writer, so streaming and connection upgrades keep working behind it.
//
// A ResponseWriter is NOT safe for concurrent use by multiple goroutines.
// Body capture is capped at [MaxCaptureBodySize] (1 MB). The first call
// to Write implicitly calls WriteHeader(200) if not already called.
type ResponseWriter struct {
	rec respwriter.Recorder
}

var responseWriterPool = sync.Pool{New: func() any { return &ResponseWriter{} }}

// NewResponseWriter obtains a [ResponseWriter] from the pool.
// If captureBody is true, it will capture up to [MaxCaptureBodySize] bytes
// of the response body. Callers must call [ResponseWriter.Release] when done.
func NewResponseWriter(w http.ResponseWriter, captureBody bool) *ResponseWriter {
	rw := responseWriterPool.Get().(*ResponseWriter) //nolint:errcheck // Type assertion is safe, pool only contains *ResponseWriter
	rw.rec.Reset(w, captureBody, MaxCaptureBodySize)
	return rw
}

// Release returns the ResponseWriter to the pool.
func (rw *ResponseWriter) Release() {
	rw.rec.Clear()
	responseWriterPool.Put(rw)
}

// Header returns the header map of the wrapped writer.
func (rw *ResponseWriter) Header() http.Header { return rw.rec.Header() }

// WriteHeader forwards the status and records it when it is final; the first
// final status wins.
func (rw *ResponseWriter) WriteHeader(statusCode int) { rw.rec.WriteHeader(statusCode) }

// Write forwards b, counting and capturing the bytes the wrapped writer
// accepted, including a partial write accompanied by an error.
func (rw *ResponseWriter) Write(b []byte) (int, error) { return rw.rec.Write(b) }

// Flush implements [http.Flusher].
func (rw *ResponseWriter) Flush() { rw.rec.Flush() }

// FlushError flushes the wrapped writer; a successful flush commits the
// implicit 200 when no final status was written yet.
func (rw *ResponseWriter) FlushError() error { return rw.rec.FlushError() }

// Hijack implements [http.Hijacker] by delegating to the wrapped writer.
func (rw *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) { return rw.rec.Hijack() }

// Push implements [http.Pusher] when the wrapped writer does.
func (rw *ResponseWriter) Push(target string, opts *http.PushOptions) error {
	return rw.rec.Push(target, opts)
}

// WriteError returns the first body write error, or nil if every write succeeded.
// A captured response with a write error must not be replayed as complete.
func (rw *ResponseWriter) WriteError() error { return rw.rec.WriteError() }

// HeadersWritten reports whether the final response headers have been sent.
// Informational responses other than 101 do not commit the final response.
func (rw *ResponseWriter) HeadersWritten() bool { return rw.rec.HeadersWritten() }

// StatusCode returns the captured status code.
func (rw *ResponseWriter) StatusCode() int { return rw.rec.StatusCode() }

// BytesWritten returns the number of body bytes written through rw, including
// any beyond the capture cap, so a caller can tell a complete capture
// (BytesWritten equals len(Body())) from a truncated one.
func (rw *ResponseWriter) BytesWritten() int64 { return rw.rec.BytesWritten() }

// Body returns the captured response body.
func (rw *ResponseWriter) Body() []byte { return rw.rec.Body() }

// BodyString returns the captured response body as a string.
func (rw *ResponseWriter) BodyString() string { return rw.rec.BodyString() }

// Unwrap returns the underlying http.ResponseWriter.
func (rw *ResponseWriter) Unwrap() http.ResponseWriter { return rw.rec.Unwrap() }
