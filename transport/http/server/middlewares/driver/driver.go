// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package driver

import (
	"context"
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
)

// ResponseInfo provides information about the HTTP response.
// It's passed to PostRequest for observability and logging.
type ResponseInfo struct {
	// StatusCode is the HTTP status code written to the response.
	StatusCode int

	// BytesWritten is the number of bytes written to the response body.
	BytesWritten int64

	// HeadersSent indicates whether headers have been sent to the client.
	HeadersSent bool
}

// Driver defines lifecycle hooks for the driven middleware pattern.
// Implementations receive callbacks before and after request processing.
// It is designed to be request-scoped, allowing state to be maintained
// across the request lifecycle.
//
// Example implementation:
//
//	type myDriver struct {
//	    startTime time.Time
//	}
//
//	func (d *myDriver) PreRequest(ctx context.Context, r *http.Request) (context.Context, error) {
//	    d.startTime = time.Now()
//	    return ctx, nil // proceed to handler
//	}
//
//	func (d *myDriver) PostRequest(ctx context.Context, resp ResponseInfo, r *http.Request, err error) {
//	    log.Printf("Request took %v, status: %d", time.Since(d.startTime), resp.StatusCode)
//	}
type Driver interface {
	// PreRequest is called before the HTTP handler execution.
	// It can modify the context or return an error to short-circuit the request.
	// If an error is returned, the request is not processed and PostRequest is still called.
	PreRequest(ctx context.Context, r *http.Request) (context.Context, error)

	// PostRequest is called after the HTTP handler execution.
	// It receives information about the response and any error from the handler.
	// This is always called, even if PreRequest returns an error.
	PostRequest(ctx context.Context, resp ResponseInfo, r *http.Request, err error)
}

// DrivenMiddleware provides the entry point for implementing the Driven Middleware pattern.
// Implementations create a request-scoped Driver for each incoming request.
//
// Example:
//
//	type myMiddleware struct {
//	    config *Config
//	}
//
//	func (m *myMiddleware) DrivenMiddleware(ctx context.Context, r *http.Request) (Driver, context.Context) {
//	    return &myDriver{config: m.config}, ctx
//	}
type DrivenMiddleware interface {
	// Name returns the middleware name for identification and ordering.
	Name() string

	// DrivenMiddleware returns a request-scoped Driver and a modified context.
	// This is called once per request to create a new Driver instance.
	DrivenMiddleware(ctx context.Context, r *http.Request) (Driver, context.Context)
}

// HTTPDrivenMiddleware wraps a DrivenMiddleware into a standard Middleware.
// This is the bridge between the driven pattern and the standard middleware interface.
//
// Example:
//
//	dm := &myDrivenMiddleware{}
//	middleware := driver.HTTPDrivenMiddleware(dm)
//	chain := middlewares.NewChain(middleware)
func HTTPDrivenMiddleware(dm DrivenMiddleware) middlewares.Middleware {
	return &drivenMiddlewareWrapper{dm: dm}
}

type drivenMiddlewareWrapper struct {
	dm DrivenMiddleware
}

func (w *drivenMiddlewareWrapper) Name() string {
	return w.dm.Name()
}

func (w *drivenMiddlewareWrapper) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Create request-scoped driver
		driver, ctx := w.dm.DrivenMiddleware(ctx, r)
		if driver == nil {
			driver = NoopDriver()
		}

		// Update request with new context
		r = r.WithContext(ctx)

		// Pre-request hook
		var preErr error
		ctx, preErr = driver.PreRequest(ctx, r)
		r = r.WithContext(ctx)

		// Wrap response writer to capture response info
		rr := &responseRecorder{
			ResponseWriter: rw,
			statusCode:     http.StatusOK, // default if WriteHeader not called
		}

		// Execute handler if PreRequest succeeded
		if preErr == nil {
			next.ServeHTTP(rr, r)
		} else {
			// If PreRequest returned an error, set 500 status
			rr.statusCode = http.StatusInternalServerError
		}

		// Post-request hook (always called)
		driver.PostRequest(ctx, ResponseInfo{
			StatusCode:   rr.statusCode,
			BytesWritten: rr.bytesWritten,
			HeadersSent:  rr.headersSent,
		}, r, preErr)
	})
}

// responseRecorder wraps http.ResponseWriter to capture response metadata.
type responseRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	headersSent  bool
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	if !r.headersSent {
		r.statusCode = statusCode
		r.headersSent = true
		r.ResponseWriter.WriteHeader(statusCode)
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.headersSent {
		r.headersSent = true
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytesWritten += int64(n)
	return n, err
}

// Unwrap returns the underlying ResponseWriter.
// This allows middleware to access the original ResponseWriter if needed.
func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// NoopDriver returns a no-op Driver.
// Useful for tests or when a middleware should do nothing for a specific request.
func NoopDriver() Driver {
	return noopDriver{}
}

type noopDriver struct{}

func (noopDriver) PreRequest(ctx context.Context, _ *http.Request) (context.Context, error) {
	return ctx, nil
}

func (noopDriver) PostRequest(context.Context, ResponseInfo, *http.Request, error) {}
