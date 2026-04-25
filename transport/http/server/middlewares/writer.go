// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"bytes"
	"net/http"
	"sync"
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
// A ResponseWriter is NOT safe for concurrent use by multiple goroutines.
// Body capture is capped at [MaxCaptureBodySize] (1 MB). The first call
// to Write implicitly calls WriteHeader(200) if not already called.
type ResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
	captureBody   bool
	body          *bytes.Buffer
}

var responseWriterPool = sync.Pool{
	New: func() any {
		return &ResponseWriter{
			statusCode: http.StatusOK,
			body:       &bytes.Buffer{},
		}
	},
}

// NewResponseWriter obtains a [ResponseWriter] from the pool.
// If captureBody is true, it will capture up to [MaxCaptureBodySize] bytes
// of the response body. Callers must call [ResponseWriter.Release] when done.
func NewResponseWriter(w http.ResponseWriter, captureBody bool) *ResponseWriter {
	rw := responseWriterPool.Get().(*ResponseWriter) //nolint:errcheck // Type assertion is safe, pool only contains *ResponseWriter
	rw.ResponseWriter = w
	rw.statusCode = http.StatusOK
	rw.headerWritten = false
	rw.captureBody = captureBody
	rw.body.Reset()
	return rw
}

// Release returns the ResponseWriter to the pool.
func (rw *ResponseWriter) Release() {
	rw.ResponseWriter = nil
	rw.body.Reset()
	responseWriterPool.Put(rw)
}

// WriteHeader captures the status code and calls the underlying WriteHeader.
func (rw *ResponseWriter) WriteHeader(statusCode int) {
	if rw.headerWritten {
		return
	}

	rw.ResponseWriter.WriteHeader(statusCode)
	rw.statusCode = statusCode
	rw.headerWritten = true
}

// Write captures the response body if enabled and calls the underlying Write.
func (rw *ResponseWriter) Write(b []byte) (int, error) {
	if !rw.headerWritten {
		rw.WriteHeader(http.StatusOK)
	}

	if rw.captureBody && rw.body != nil && rw.body.Len() < MaxCaptureBodySize {
		remaining := MaxCaptureBodySize - rw.body.Len()
		if len(b) <= remaining {
			rw.body.Write(b)
		} else {
			rw.body.Write(b[:remaining])
		}
	}

	return rw.ResponseWriter.Write(b)
}

// StatusCode returns the captured status code.
func (rw *ResponseWriter) StatusCode() int {
	return rw.statusCode
}

// Body returns the captured response body.
func (rw *ResponseWriter) Body() []byte {
	return rw.body.Bytes()
}

// BodyString returns the captured response body as a string.
func (rw *ResponseWriter) BodyString() string {
	return rw.body.String()
}

// Unwrap returns the underlying http.ResponseWriter.
func (rw *ResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
