// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package responder

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/internal/transport/respwriter"
)

// maxCapturedBody caps the error body kept as diagnostic text for the
// rewritten response; the rest is discarded. A buffer that grew past it is
// not returned to the pool.
const maxCapturedBody = 64 << 10

// ErrorInterceptor wraps [http.ResponseWriter] to intercept error responses
// (status codes >= 400) and rewrite them using the configured [ErrorWriter].
// Create instances with [NewErrorInterceptor].
//
// For non-error status codes, writes pass through directly to the underlying
// writer. For error codes, both the status and body are buffered until
// [ErrorInterceptor.Finish] is called, at which point the buffered message
// is converted to a structured error response. A response the writer package
// already structured (see [respwriter.MarkStructured]) passes through as is.
// [ErrorInterceptor.Flush] implements [http.Flusher] for streaming responses
// and never ends the request.
//
// The first final status wins: informational statuses other than 101 are
// forwarded and do not count, a later status after the first final one is
// ignored. Only the first 64 KiB of a buffered error body are kept, and the
// handler's Content-Length, Content-Encoding and ETag are dropped when the
// body is rewritten.
//
// ErrorInterceptor is NOT safe for concurrent use. It is designed for
// single-request, single-goroutine usage within middleware chains.
// Instances are pooled via [sync.Pool]. After [ErrorInterceptor.Finish],
// the interceptor is returned to the pool and must not be used again.
type ErrorInterceptor struct {
	http.ResponseWriter
	request     *http.Request
	writer      ErrorWriter
	statusCode  int
	buffer      bytes.Buffer
	intercepted bool
	// final reports whether the first final status was accepted, whether it
	// was forwarded or buffered.
	final bool
	// written reports whether the response reached the underlying writer.
	written bool
	// structured is set by [ErrorInterceptor.MarkStructured] for a response
	// the writer already formatted.
	structured bool
}

var _ respwriter.StructuredMarker = (*ErrorInterceptor)(nil)

var interceptorPool = sync.Pool{
	New: func() any {
		return &ErrorInterceptor{}
	},
}

// NewErrorInterceptor creates a new ErrorInterceptor that wraps the given
// ResponseWriter and uses the provided ErrorWriter for structured error responses.
//
// The caller must call Finish() when request handling is complete to ensure
// any buffered error responses are written.
//
// Example:
//
//	interceptor := responder.NewErrorInterceptor(w, r, writer)
//	defer interceptor.Finish()
//	next.ServeHTTP(interceptor, r)
func NewErrorInterceptor(w http.ResponseWriter, r *http.Request, writer ErrorWriter) *ErrorInterceptor {
	e := interceptorPool.Get().(*ErrorInterceptor) //nolint:errcheck // Type assertion from pool is safe by design
	e.ResponseWriter = w
	e.request = r
	e.writer = writer
	e.statusCode = 0
	e.buffer.Reset()
	e.intercepted = false
	e.final = false
	e.written = false
	e.structured = false
	return e
}

// MarkStructured implements [respwriter.StructuredMarker]: the response about
// to be written is already a structured error, so it is forwarded as is.
func (e *ErrorInterceptor) MarkStructured() { e.structured = true }

// WriteHeader intercepts error status codes (>= 400) and buffers them
// instead of writing directly to the underlying ResponseWriter. The first
// final status wins.
func (e *ErrorInterceptor) WriteHeader(code int) {
	if e.final {
		return
	}

	if !respwriter.IsFinal(code) {
		e.ResponseWriter.WriteHeader(code)
		return
	}
	e.final = true

	if code >= http.StatusBadRequest && !e.structured {
		e.statusCode = code
		e.intercepted = true
		return // Don't write yet, wait for body
	}

	e.ResponseWriter.WriteHeader(code)
	e.written = true
}

// Write intercepts the response body when an error status was set.
// For error responses, the body is buffered for later processing.
func (e *ErrorInterceptor) Write(b []byte) (int, error) {
	if e.intercepted {
		if remaining := maxCapturedBody - e.buffer.Len(); remaining > 0 {
			e.buffer.Write(b[:min(len(b), remaining)])
		}
		return len(b), nil
	}

	// If WriteHeader wasn't called, this is a 200 OK
	e.final = true
	e.written = true

	// CodeQL: False positive - ErrorInterceptor is a transparent middleware proxy that
	// conditionally buffers error responses for custom formatting. For non-error responses,
	// it forwards bytes directly from the handler without modification. XSS prevention must
	// be done in the handlers that generate content, not in infrastructure middleware.
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter
	return e.ResponseWriter.Write(b) //nolint:gosec // Transparent proxy for non-error responses
}

// Finish writes any buffered error response using the ErrorWriter.
// This method must be called when request handling is complete.
// After Finish, the interceptor is returned to the pool and must not be used.
func (e *ErrorInterceptor) Finish() {
	defer e.release()

	if !e.intercepted {
		return
	}

	// Create error from buffered message
	message := strings.TrimSpace(e.buffer.String())
	if message == "" {
		message = http.StatusText(e.statusCode)
	}

	err := errors.New(message)

	// The rewritten body has its own length, encoding and validators.
	h := e.Header()
	h.Del("Content-Length")
	h.Del("Content-Encoding")
	h.Del("ETag")

	// Write structured error response
	_ = e.writer.WriteError(e.ResponseWriter, e.request, err, e.statusCode) //nolint:errcheck // Best effort, response already committed
}

// release returns the interceptor to the pool.
func (e *ErrorInterceptor) release() {
	e.ResponseWriter = nil
	e.request = nil
	e.writer = nil
	if e.buffer.Cap() > maxCapturedBody {
		e.buffer = bytes.Buffer{}
	} else {
		e.buffer.Reset()
	}
	interceptorPool.Put(e)
}

// Unwrap returns the underlying ResponseWriter.
// This is useful for middleware that need access to the original writer.
func (e *ErrorInterceptor) Unwrap() http.ResponseWriter {
	return e.ResponseWriter
}

// Hijack implements http.Hijacker interface for WebSocket support.
func (e *ErrorInterceptor) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := e.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
}

// Flush implements [http.Flusher]; see [ErrorInterceptor.FlushError].
func (e *ErrorInterceptor) Flush() { _ = e.FlushError() } //nolint:errcheck // http.Flusher has no error result

// FlushError forwards a flush to the underlying writer and commits the
// response. While an error response is being buffered it is a no-op, so
// [ErrorInterceptor.Finish] can still write it. Flush does not end the
// request.
func (e *ErrorInterceptor) FlushError() error {
	if e.intercepted {
		return nil
	}
	err := http.NewResponseController(e.ResponseWriter).Flush()
	// A failed flush may already have sent the status, so only a writer that
	// cannot flush at all leaves the response open.
	if err == nil || !errors.Is(err, http.ErrNotSupported) {
		e.final = true
		e.written = true
	}
	return err
}

// Push implements http.Pusher interface for HTTP/2 server push.
func (e *ErrorInterceptor) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := e.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
