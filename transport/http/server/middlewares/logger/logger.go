// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/core/time/timeformat"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/observability"

	coreio "github.com/altessa-s/go-atlas/core/io"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

const (
	// DefaultFieldsCapacity is the default capacity for pre-allocated log fields
	// passed to [LogHandler.Log]. This reduces allocations for typical request
	// logging scenarios where roughly 15 fields are collected (method, path,
	// proto, peer_ip, user_agent, real_ip, status, start_time, end_time,
	// duration, request_id, and optional request/response content).
	DefaultFieldsCapacity = 15
)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	logHandler       LogHandler
	opts             *options
	ignoreMethodsMap map[string]struct{}
	logCodesSet      map[int]struct{}
	ignoreCodesSet   map[int]struct{}
}

// Dependencies returns middlewares that logger reads from context.
// All dependencies are optional for ordering - logger gracefully degrades
// if requestid, realip, or tracing are not available in context.
func (m *middleware) Dependencies() []string {
	return []string{"requestid", "realip", "tracing"}
}

// Handler wraps an http.Handler with request logging functionality.
func (m *middleware) Handler(next http.Handler) http.Handler {
	if m.logHandler == nil {
		return next
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Check if this path should be ignored
		if m.ShouldIgnore(request.URL.Path) {
			next.ServeHTTP(writer, request)
			return
		}

		// Check if this HTTP method should be ignored (O(1) lookup)
		if len(m.ignoreMethodsMap) > 0 {
			if _, ignored := m.ignoreMethodsMap[corestrings.InternUpperString(request.Method)]; ignored {
				next.ServeHTTP(writer, request)
				return
			}
		}

		// Capture request body if enabled.
		// Uses a pooled buffer to avoid per-request allocations from io.ReadAll.
		var requestBody string
		if m.opts.logRequest && request.Body != nil {
			buf := coreio.GetBuffer()
			_, readErr := io.CopyN(buf, request.Body, int64(middlewares.MaxCaptureBodySize))
			if readErr == nil || errors.Is(readErr, io.EOF) {
				// Convert to string once (single allocation), then use
				// strings.NewReader for body restoration (no extra copy).
				requestBody = string(buf.Bytes())
				request.Body = io.NopCloser(strings.NewReader(requestBody))
			}
			coreio.PutBuffer(buf)
		}

		start := time.Now()

		// Pre-allocate fields with expected capacity
		fields := make(slogx.Fields, 0, DefaultFieldsCapacity)
		fields = append(fields,
			slogx.Field{Key: FieldKeyHTTPMethod, Value: request.Method},
			slogx.Field{Key: FieldKeyHTTPPath, Value: request.URL.Path},
			slogx.Field{Key: FieldKeyHTTPProto, Value: request.Proto},
		)

		// Extract peer IP
		peerIP, _, splitErr := net.SplitHostPort(request.RemoteAddr)
		if splitErr != nil {
			peerIP = request.RemoteAddr
		}
		fields = append(fields,
			slogx.Field{Key: observability.FieldKeyClientPeerIP, Value: peerIP},
			slogx.Field{Key: observability.FieldKeyClientUserAgent, Value: request.UserAgent()},
		)

		// Extract real IP if available from context (set by realip middleware).
		// If realip middleware is not used, FromContext returns invalid netip.Addr
		// and this field is simply not added to the log (graceful degradation).
		if realIP := clientip.FromContext(request.Context()); realIP.IsValid() {
			fields = append(fields, slogx.Field{Key: observability.FieldKeyClientRealIP, Value: realIP.String()})
		}

		// Inject fields into context
		ctx := slogx.InjectFields(request.Context(), fields)

		// Inject enriched logger into context if configured
		if m.opts.contextLogger != nil {
			ctx = slogx.InjectLogger(ctx, m.opts.contextLogger)
		}

		request = request.WithContext(ctx)

		// Wrap the writer to capture the status code and optionally the response body
		sw := middlewares.NewResponseWriter(writer, m.opts.logResponse)
		defer sw.Release()
		writer = sw

		next.ServeHTTP(writer, request)

		// Get status code
		status := http.StatusOK
		var responseBody string
		if sw, ok := writer.(*middlewares.ResponseWriter); ok {
			status = sw.StatusCode()
			if m.opts.logResponse {
				responseBody = sw.BodyString()
			}
		}

		// Check if this status code should be logged (O(1) lookup)
		if !shouldLogStatusCode(status, m.logCodesSet, m.ignoreCodesSet) {
			return
		}

		// Add timing fields
		now := time.Now()
		elapsed := now.Sub(start)

		// Get fields from context (may have been modified by handlers)
		fields = slogx.FieldsFromContext(ctx)
		fields = append(fields,
			slogx.Field{Key: FieldKeyHTTPStatus, Value: status},
			slogx.Field{Key: observability.FieldKeyRequestStartTime, Value: timeformat.FormatTime(start, m.opts.timeFormat)},
			slogx.Field{Key: observability.FieldKeyRequestEndTime, Value: timeformat.FormatTime(now, m.opts.timeFormat)},
			slogx.Field{Key: observability.FieldKeyRequestDuration, Value: timeformat.FormatDuration(elapsed, m.opts.timeFormat)},
		)

		// Add request ID
		if reqID := requestid.FromContext(ctx); reqID != "" {
			fields = append(fields, slogx.Field{Key: observability.FieldKeyRequestID, Value: reqID})
		}

		// Add trace correlation fields if available
		if traceID := tracing.TraceIDFromContext(ctx); traceID != "" {
			fields = append(fields, slogx.Field{Key: observability.FieldKeyTraceID, Value: traceID})
		}
		if spanID := tracing.SpanIDFromContext(ctx); spanID != "" {
			fields = append(fields, slogx.Field{Key: observability.FieldKeySpanID, Value: spanID})
		}

		// Add request body if captured
		if requestBody != "" {
			if m.opts.bodyRedactor != nil {
				requestBody = m.opts.bodyRedactor(requestBody)
			}
			fields = append(fields, slogx.Field{Key: observability.FieldKeyRequestContent, Value: requestBody})
		}

		// Add response body if captured
		if responseBody != "" {
			if m.opts.bodyRedactor != nil {
				responseBody = m.opts.bodyRedactor(responseBody)
			}
			fields = append(fields, slogx.Field{Key: observability.FieldKeyResponseContent, Value: responseBody})
		}

		m.logHandler.Log(ctx, "request completed", status, fields)
	})
}

// New creates a new logger middleware with the given [LogHandler] and options.
// If logHandler is nil, the returned middleware is a no-op pass-through.
//
// This is the recommended way to create the logger middleware.
// For [slog.Logger], use: logger.New(logger.Slog(myLogger), opts...)
func New(logHandler LogHandler, opt ...Option) *middleware {
	if logHandler == nil {
		return &middleware{
			BaseMiddleware: middlewares.NewBaseMiddleware("logger", nil),
		}
	}

	opts := newOptions(opt...)

	// Build ignore methods map for O(1) lookup
	ignoreMethodsMap := make(map[string]struct{}, len(opts.ignoreMethods))
	for _, method := range opts.ignoreMethods {
		// Methods are already uppercased by optgen, but intern for consistency
		ignoreMethodsMap[corestrings.InternUpperString(method)] = struct{}{}
	}

	// Build log codes set for O(1) lookup (custom codes or default)
	var logCodesSet map[int]struct{}
	if len(opts.logResponseCodes) > 0 {
		logCodesSet = make(map[int]struct{}, len(opts.logResponseCodes))
		for _, code := range opts.logResponseCodes {
			logCodesSet[code] = struct{}{}
		}
	} else {
		logCodesSet = DefaultLogStatusCodesSet
	}

	// Build ignore codes set for O(1) lookup
	ignoreCodesSet := make(map[int]struct{}, len(opts.ignoreResponseCodes))
	for _, code := range opts.ignoreResponseCodes {
		ignoreCodesSet[code] = struct{}{}
	}

	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			"logger",
			opts.ignorePaths,
			opts.ignorePatterns,
			nil,
		),
		logHandler:       logHandler,
		opts:             opts,
		ignoreMethodsMap: ignoreMethodsMap,
		logCodesSet:      logCodesSet,
		ignoreCodesSet:   ignoreCodesSet,
	}
}

// Middleware defines a middleware that logs HTTP requests with configurable options.
// It uses the LogHandler interface for flexibility, allowing different logging backends.
//
// This is a convenience function; prefer New() for access to the full Middleware interface.
// For slog.Logger, use: logger.Middleware(logger.Slog(myLogger), opts...)
//
// Example usage:
//
//	// Using LogHandler interface
//	middleware := logger.Middleware(logger.Slog(myLogger),
//	    logger.WithTimeFormat(logger.TimeFormatUnixMilli),
//	)
//
//	// Production mode
//	middleware := logger.Middleware(logger.Slog(myLogger),
//	    logger.WithIgnoreResponseCodes(200, 204, 304),
//	    logger.WithIgnorePaths("/health", "/ready"),
//	    logger.WithIgnoreMethods("OPTIONS", "HEAD"),
//	)
func Middleware(logHandler LogHandler, opt ...Option) func(next http.Handler) http.Handler {
	return New(logHandler, opt...).Handler
}

// shouldLogStatusCode determines if a status code should be logged based on pre-computed sets.
// Both lookups are O(1) for optimal performance in the hot path.
func shouldLogStatusCode(status int, logCodesSet, ignoreCodesSet map[int]struct{}) bool {
	// Check ignored codes first (takes precedence) - O(1) lookup
	if _, ignored := ignoreCodesSet[status]; ignored {
		return false
	}

	// Check if in log codes - O(1) lookup
	_, shouldLog := logCodesSet[status]
	return shouldLog
}
