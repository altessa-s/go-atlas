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
)

// ErrorInterceptor wraps [http.ResponseWriter] to intercept error responses
// (status codes >= 400) and rewrite them using the configured [ErrorWriter].
// Create instances with [NewErrorInterceptor].
//
// For non-error status codes, writes pass through directly to the underlying
// writer. For error codes, both the status and body are buffered until
// [ErrorInterceptor.Flush] is called, at which point the buffered message
// is converted to a structured error response.
//
// ErrorInterceptor is NOT safe for concurrent use. It is designed for
// single-request, single-goroutine usage within middleware chains.
// Instances are pooled via [sync.Pool]. After [ErrorInterceptor.Flush],
// the interceptor is returned to the pool and must not be used again.
type ErrorInterceptor struct {
	http.ResponseWriter
	request     *http.Request
	writer      ErrorWriter
	statusCode  int
	buffer      bytes.Buffer
	intercepted bool
	written     bool
}

var interceptorPool = sync.Pool{
	New: func() any {
		return &ErrorInterceptor{}
	},
}

// NewErrorInterceptor creates a new ErrorInterceptor that wraps the given
// ResponseWriter and uses the provided ErrorWriter for structured error responses.
//
// The caller must call Flush() when request handling is complete to ensure
// any buffered error responses are written.
//
// Example:
//
//	interceptor := responder.NewErrorInterceptor(w, r, writer)
//	defer interceptor.Flush()
//	next.ServeHTTP(interceptor, r)
func NewErrorInterceptor(w http.ResponseWriter, r *http.Request, writer ErrorWriter) *ErrorInterceptor {
	e := interceptorPool.Get().(*ErrorInterceptor) //nolint:errcheck // Type assertion from pool is safe by design
	e.ResponseWriter = w
	e.request = r
	e.writer = writer
	e.statusCode = 0
	e.buffer.Reset()
	e.intercepted = false
	e.written = false
	return e
}

// WriteHeader intercepts error status codes (>= 400) and buffers them
// instead of writing directly to the underlying ResponseWriter.
func (e *ErrorInterceptor) WriteHeader(code int) {
	if e.written {
		return
	}

	if code >= http.StatusBadRequest {
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
		return e.buffer.Write(b)
	}

	// If WriteHeader wasn't called, this is a 200 OK
	if !e.written {
		e.written = true
	}

	return e.ResponseWriter.Write(b)
}

// Flush writes any buffered error response using the ErrorWriter.
// This method must be called when request handling is complete.
// After Flush, the interceptor is returned to the pool and must not be used.
func (e *ErrorInterceptor) Flush() {
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

	// Write structured error response
	_ = e.writer.WriteError(e.ResponseWriter, e.request, err, e.statusCode) //nolint:errcheck // Best effort, response already committed
}

// release returns the interceptor to the pool.
func (e *ErrorInterceptor) release() {
	e.ResponseWriter = nil
	e.request = nil
	e.writer = nil
	e.buffer.Reset()
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

// FlushHTTP provides flusher support for streaming responses.
func (e *ErrorInterceptor) FlushHTTP() {
	if flusher, ok := e.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Push implements http.Pusher interface for HTTP/2 server push.
func (e *ErrorInterceptor) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := e.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
