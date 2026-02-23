// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import "net/http"

// recorder is a wrapper around http.ResponseWriter that captures the
// status code and response size written to the response.
type recorder struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
	size          int
}

// newRecorder creates a new recorder wrapping the provided ResponseWriter.
func newRecorder(w http.ResponseWriter) *recorder {
	return &recorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

// WriteHeader captures the status code before writing it to the response.
func (r *recorder) WriteHeader(statusCode int) {
	if r.headerWritten {
		return
	}

	r.ResponseWriter.WriteHeader(statusCode)
	r.statusCode = statusCode
	r.headerWritten = true
}

// Write captures the response size while writing to the response.
func (r *recorder) Write(b []byte) (int, error) {
	r.headerWritten = true

	size, err := r.ResponseWriter.Write(b)
	if err == nil {
		r.size += size
	}

	return size, err
}

// StatusCode returns the captured HTTP status code.
func (r *recorder) StatusCode() int {
	return r.statusCode
}

// Size returns the total response body size in bytes.
func (r *recorder) Size() int {
	return r.size
}

// Unwrap returns the underlying ResponseWriter for middleware compatibility.
func (r *recorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
