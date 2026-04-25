// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/altessa-s/go-atlas/transport/http/server/codec"
	"github.com/altessa-s/go-atlas/transport/internal/headers"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreio "github.com/altessa-s/go-atlas/core/io"
)

// Sentinel errors for [Writer] and [ReadWriter] operations.
var (
	// ErrNoCodecAvailable is returned by [Writer.Read] and [Writer.Write] when
	// no codec in the [codec.Registry] matches the request Content-Type or
	// Accept header.
	ErrNoCodecAvailable = errors.New("no codec available for content type")

	// ErrReadBody is returned by [Writer.Read] (and [ReadWriter.Read]) when
	// the request body cannot be read from the underlying [io.Reader].
	ErrReadBody = errors.New("error reading request body")

	// ErrUnmarshal is returned by [Writer.Read] (and [ReadWriter.Read]) when
	// the codec fails to decode the request body into the target value.
	ErrUnmarshal = errors.New("error unmarshaling request body")

	// ErrNilResponseWriter is returned by [Writer.Write], [Writer.WriteError],
	// and [Writer.WriteStream] when the http.ResponseWriter argument is nil.
	ErrNilResponseWriter = errors.New("http.ResponseWriter cannot be nil")

	// ErrNilRequest is returned by [Writer.Read], [Writer.Write], [Writer.WriteError],
	// and [Writer.WriteStream] when the *http.Request argument is nil.
	ErrNilRequest = errors.New("http.Request cannot be nil")

	// ErrNilOutput is returned by [Writer.Read] (and [ReadWriter.Read]) when
	// the output parameter is nil.
	ErrNilOutput = errors.New("output parameter cannot be nil")

	// ErrBodySizeLimitExceeded is returned by [Writer.Read] when the request
	// body exceeds the limit configured via [WithMaxBodySize].
	ErrBodySizeLimitExceeded = errors.New("request body size limit exceeded")
)

// Writer handles HTTP response writing with content negotiation.
// Create instances with [New]. The response envelope is determined by the
// configured [Builder] (default [Default]).
//
// Writer is safe for concurrent use from multiple goroutines. Each method
// operates independently on its input parameters and the underlying
// [codec.Registry] is also thread-safe. However, callers must ensure that
// the http.ResponseWriter and *http.Request for a single request are not
// accessed concurrently.
type Writer struct {
	options *options
}

// New creates a new [Writer] with the given options. If no options are
// specified, the [codec.DefaultRegistry] and [Default] response builder are used.
func New(opts ...Option) *Writer {
	return &Writer{options: newOptions(opts...)}
}

// Write writes a response with content negotiation.
// Returns ErrNilResponseWriter or ErrNilRequest if arguments are nil.
func (wr *Writer) Write(w http.ResponseWriter, r *http.Request, data any) error {
	if w == nil {
		return ErrNilResponseWriter
	}
	if r == nil {
		return ErrNilRequest
	}

	structuredResponse, statusCode := wr.options.responseBuilder.Build(r, data)

	encoder, mimeType, err := wr.negotiateEncoder(r)
	if err != nil {
		return wr.writeError(w, r, err, http.StatusNotAcceptable)
	}

	// Encode response
	body, err := encoder.Encode(structuredResponse)
	if err != nil {
		return wr.writeError(w, r, err, http.StatusInternalServerError)
	}

	// Write response (Content-Length is set automatically by http.ResponseWriter)
	w.Header().Set(headers.ContentType, mimeType+"; charset=utf-8")
	w.WriteHeader(statusCode)

	if _, err = w.Write(body); err != nil {
		return coreerrs.WrapOperation(err, "write response body")
	}

	return nil
}

// Read reads and decodes request body into out.
// Returns ErrNilRequest if request is nil, ErrNilOutput if out is nil,
// ErrBodySizeLimitExceeded if body exceeds configured maxBodySize.
func (wr *Writer) Read(r *http.Request, out any) error {
	if r == nil {
		return ErrNilRequest
	}
	if out == nil {
		return ErrNilOutput
	}

	// Get Content-Type
	contentType := r.Header.Get(headers.ContentType)
	if contentType == "" {
		contentType = wr.options.defaultCodec
	}

	// Get decoder
	decoder, ok := wr.options.registry.GetDecoder(contentType)
	if !ok {
		return coreerrs.Wrapf(ErrNoCodecAvailable, contentType)
	}

	// Read body with optional size limit
	body, err := wr.readBody(r)
	if err != nil {
		return err
	}

	// Decode
	if err = decoder.Decode(body, out); err != nil {
		return errors.Join(ErrUnmarshal, err)
	}

	return nil
}

// readBody reads the request body with optional size limiting.
// Uses a pooled bytes.Buffer to avoid repeated growth allocations on every request.
func (wr *Writer) readBody(r *http.Request) ([]byte, error) {
	buf := coreio.GetBuffer()
	defer coreio.PutBuffer(buf)

	if wr.options.maxBodySize > 0 {
		limitedReader := coreio.NewLimitedReadCloser(r.Body, wr.options.maxBodySize)
		defer func() { _ = limitedReader.Close() }()

		if _, err := buf.ReadFrom(limitedReader); err != nil {
			if errors.Is(err, coreio.ErrReadLimitExceeded) {
				return nil, ErrBodySizeLimitExceeded
			}
			return nil, errors.Join(ErrReadBody, err)
		}
		return bytes.Clone(buf.Bytes()), nil
	}

	if _, err := buf.ReadFrom(r.Body); err != nil {
		return nil, errors.Join(ErrReadBody, err)
	}
	return bytes.Clone(buf.Bytes()), nil
}

// WriteError writes an error response with optional status code.
// If no status code is provided, defaults to 500 Internal Server Error.
func (wr *Writer) WriteError(w http.ResponseWriter, r *http.Request, err error, statusCode ...int) error {
	status := http.StatusInternalServerError
	if len(statusCode) > 0 {
		status = statusCode[0]
	}

	return wr.writeError(w, r, err, status)
}

// negotiateEncoder selects encoder based on Accept header.
func (wr *Writer) negotiateEncoder(r *http.Request) (codec.Encoder, string, error) {
	acceptHeader := r.Header.Get(headers.Accept)

	// Try content negotiation
	if acceptHeader != "" && acceptHeader != "*/*" {
		encoder, mimeType, err := wr.options.registry.Negotiate(acceptHeader)
		if err == nil {
			return encoder, mimeType, nil
		}

		// If fallback is disabled, return the error
		if !wr.options.fallbackOnNegotiationError {
			return nil, "", coreerrs.Wrapf(codec.ErrNoCodecFound, acceptHeader)
		}
	}

	// Fallback to default codec
	encoder, ok := wr.options.registry.GetEncoder(wr.options.defaultCodec)
	if !ok {
		return nil, "", coreerrs.Wrapf(ErrNoCodecAvailable, wr.options.defaultCodec)
	}

	return encoder, wr.options.defaultCodec, nil
}

// writeError writes error response with fallback handling.
func (wr *Writer) writeError(w http.ResponseWriter, r *http.Request, err error, sc ...int) error {
	// Build error response
	errorResponse, statusCode := wr.options.responseBuilder.Build(r, err)

	if len(sc) > 0 && sc[0] > 0 {
		statusCode = sc[0]
	}

	// Get encoder
	encoder, mimeType, encErr := wr.negotiateEncoder(r)
	if encErr != nil {
		// Cannot negotiate codec, write plain text error
		w.Header().Set(headers.ContentType, ContentTypePlainText)
		w.WriteHeader(http.StatusInternalServerError)
		wr.writeFallback(r.Context(), w)
		return coreerrs.WrapOperation(encErr, "negotiate encoder for error response")
	}

	// Encode error response
	body, encErr := encoder.Encode(errorResponse)
	if encErr != nil {
		// Cannot encode error, write plain text
		w.Header().Set(headers.ContentType, ContentTypePlainText)
		w.WriteHeader(http.StatusInternalServerError)
		wr.writeFallback(r.Context(), w)
		return coreerrs.WrapOperation(encErr, "encode error response")
	}

	// Write error response (Content-Length is set automatically)
	w.Header().Set(headers.ContentType, mimeType+"; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body) //nolint:errcheck // Best effort for error response

	return nil
}

// writeFallback writes the default fallback message and logs any write errors.
func (wr *Writer) writeFallback(ctx context.Context, w http.ResponseWriter) {
	if _, err := w.Write([]byte(DefaultFallbackMessage)); err != nil {
		if wr.options.logger != nil {
			wr.options.logger.WarnContext(ctx, "fallback response write failed", "error", err)
		}
	}
}

// ReadWriter combines request reading and response writing into a per-request
// helper. It provides a convenient API for handlers without needing to pass
// http.ResponseWriter and *http.Request to each method.
//
// Create instances via [Writer.NewReadWriter] or [NewReadWriter]. Instances are
// drawn from a [sync.Pool] and must be returned by calling [ReadWriter.Release]
// when the handler is done.
//
// ReadWriter is NOT safe for concurrent use. Each instance must be used only
// within a single HTTP request by a single goroutine.
type ReadWriter interface {
	// Request returns the original *http.Request.
	// Use for accessing URL parameters, headers, context, etc.
	Request() *http.Request

	// ResponseWriter returns the original http.ResponseWriter.
	// Use for low-level operations: setting headers, streaming,
	// Server-Sent Events, etc.
	ResponseWriter() http.ResponseWriter

	// Read decodes the request body into out.
	// Codec is selected based on the Content-Type header.
	// out must be a pointer to a struct for decoding.
	Read(out any) error

	// Write encodes data and writes the response.
	// Codec is selected based on the Accept header (content negotiation).
	Write(data any) error

	// WriteError writes a structured error response.
	// statusCode optionally overrides the HTTP status code.
	WriteError(err error, statusCode ...int) error

	// WriteStream writes a streaming response without buffering.
	// Use for large responses that should not be buffered in memory.
	WriteStream(data any) error

	// Release releases the ReadWriter back to the pool.
	// This should be called when the request handling is complete.
	Release()
}

// readWriter implements ReadWriter interface.
type readWriter struct {
	w      http.ResponseWriter
	r      *http.Request
	writer *Writer
}

var readWriterPool = sync.Pool{
	New: func() any {
		return &readWriter{}
	},
}

// NewReadWriter creates a ReadWriter for handling a request.
//
// Parameters:
//   - w: http.ResponseWriter for writing the response
//   - r: *http.Request for reading the request
//   - wr: *Writer with configured codecs and options
//
// Example:
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//	    rw := writers.NewReadWriter(w, r, sharedWriter)
//	    defer rw.Release()
//	    var input RequestBody
//	    if err := rw.Read(&input); err != nil {
//	        rw.WriteError(err, http.StatusBadRequest)
//	        return
//	    }
//	    rw.Write(response)
//	}
func NewReadWriter(w http.ResponseWriter, r *http.Request, wr *Writer) ReadWriter {
	rw := readWriterPool.Get().(*readWriter) //nolint:errcheck // Type assertion from pool is safe by design
	rw.w = w
	rw.r = r
	rw.writer = wr
	return rw
}

// NewReadWriter creates a ReadWriter helper for the given request.
// This method satisfies the http.ResponseWriter interface.
func (wr *Writer) NewReadWriter(w http.ResponseWriter, r *http.Request) ReadWriter {
	return NewReadWriter(w, r, wr)
}

func (rw *readWriter) Request() *http.Request {
	return rw.r
}

func (rw *readWriter) ResponseWriter() http.ResponseWriter {
	return rw.w
}

func (rw *readWriter) Read(out any) error {
	return rw.writer.Read(rw.r, out)
}

func (rw *readWriter) Write(data any) error {
	return rw.writer.Write(rw.w, rw.r, data)
}

func (rw *readWriter) WriteError(err error, statusCode ...int) error {
	return rw.writer.WriteError(rw.w, rw.r, err, statusCode...)
}

func (rw *readWriter) WriteStream(data any) error {
	return rw.writer.WriteStream(rw.w, rw.r, data)
}

func (rw *readWriter) Release() {
	rw.w = nil
	rw.r = nil
	rw.writer = nil
	readWriterPool.Put(rw)
}
