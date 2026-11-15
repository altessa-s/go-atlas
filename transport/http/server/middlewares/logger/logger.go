// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"
	"github.com/altessa-s/go-atlas-core/time/timeformat"
	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/internal/transport/observability"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
	coreio "github.com/altessa-s/go-atlas-core/io"
	corestrings "github.com/altessa-s/go-atlas-core/text/strings"
	tracingmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/tracing"
	slogx "github.com/altessa-s/go-slogx"
)

const (
	// DefaultFieldsCapacity is the default capacity for pre-allocated log fields
	// passed to [LogHandler.Log]. This reduces allocations for typical request
	// logging scenarios where roughly 15 fields are collected (method, path,
	// proto, peer_ip, user_agent, real_ip, status, start_time, end_time,
	// duration, request_id, and optional request/response content).
	DefaultFieldsCapacity = 15
)

const middlewareName = "logger"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	logHandler       LogHandler
	opts             *options
	ignoreMethodsMap map[string]struct{}
	logCodesSet      map[int]struct{}
	ignoreCodesSet   map[int]struct{}
	redactQuery      *coremaps.ImmutableMap[string, struct{}]
}

// Dependencies returns middlewares that logger reads from context.
// All dependencies are optional for ordering - logger gracefully degrades
// if requestid, realip, or tracing are not available in context.
func (m *middleware) Dependencies() []string {
	return []string{requestid.Name(), realip.Name(), tracingmw.Name()}
}

// Handler wraps an http.Handler with request logging functionality.
func (m *middleware) Handler(next http.Handler) http.Handler {
	if m.logHandler == nil {
		return next
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if m.skipRequest(request) {
			next.ServeHTTP(writer, request)
			return
		}
		m.serve(next, writer, request)
	})
}

// skipRequest reports whether the request's path or method is excluded from
// logging.
func (m *middleware) skipRequest(request *http.Request) bool {
	// Check if this path should be ignored
	if m.ShouldIgnoreRequest(request) {
		return true
	}
	// Check if this HTTP method should be ignored (O(1) lookup)
	if len(m.ignoreMethodsMap) == 0 {
		return false
	}
	_, ignored := m.ignoreMethodsMap[strings.ToUpper(request.Method)] // Client-written: not interned.
	return ignored
}

// serve runs next with the request fields in the context and logs the
// completed request.
func (m *middleware) serve(next http.Handler, writer http.ResponseWriter, request *http.Request) {
	// Capture request body if enabled.
	// Uses a pooled buffer to avoid per-request allocations from io.ReadAll.
	var requestBody string
	if m.opts.logRequest && request.Body != nil {
		request.Body, requestBody = captureBody(request.Body)
	}

	start := time.Now()

	// Inject fields into context
	ctx := slogx.InjectFields(request.Context(), m.requestFields(request))

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
	rw, isRW := writer.(*middlewares.ResponseWriter)
	if isRW {
		status = rw.StatusCode()
	}

	// Check if this status code should be logged (O(1) lookup)
	if !shouldLogStatusCode(status, m.logCodesSet, m.ignoreCodesSet) {
		return
	}
	// Skip building the completion record when the handler would drop it.
	if lc, ok := m.logHandler.(levelChecker); ok && !lc.Enabled(ctx, status) {
		return
	}
	var responseBody string
	if isRW && m.opts.logResponse {
		responseBody = rw.BodyString()
	}

	m.logCompletion(ctx, status, start, requestBody, responseBody)
}

// requestFields returns the fields describing the request, logged with every
// record of its context.
func (m *middleware) requestFields(request *http.Request) slogx.Fields {
	// Pre-allocate fields with expected capacity
	fields := make(slogx.Fields, 0, DefaultFieldsCapacity)
	fields = append(fields,
		slogx.Field{Key: FieldKeyHTTPMethod, Value: request.Method},
		slogx.Field{Key: FieldKeyHTTPPath, Value: sanitizeLogValue(request.URL.Path, m.opts.maxPathLength)},
		slogx.Field{Key: FieldKeyHTTPProto, Value: request.Proto},
	)
	if m.opts.logQuery && request.URL.RawQuery != "" {
		fields = append(fields, slogx.Field{Key: FieldKeyHTTPQuery, Value: m.redactedQuery(request.URL.RawQuery)})
	}

	// Extract peer IP
	peerIP, _, splitErr := net.SplitHostPort(request.RemoteAddr)
	if splitErr != nil {
		peerIP = request.RemoteAddr
	}
	fields = append(fields,
		slogx.Field{Key: observability.FieldKeyClientPeerIP, Value: peerIP},
		slogx.Field{
			Key:   observability.FieldKeyClientUserAgent,
			Value: sanitizeLogValue(request.UserAgent(), m.opts.maxUserAgentLength),
		},
	)

	// Extract real IP if available from context (set by realip middleware).
	// If realip middleware is not used, FromContext returns invalid netip.Addr
	// and this field is simply not added to the log (graceful degradation).
	if realIP := clientip.FromContext(request.Context()); realIP.IsValid() {
		fields = append(fields, slogx.Field{Key: observability.FieldKeyClientRealIP, Value: realIP.String()})
	}
	return fields
}

// sanitizeLogValue prepares a client-controlled string for a log record: it
// is cut to maxLen bytes and its control characters (CR, LF, the rest of C0,
// DEL and C1) are removed, so a decoded path such as "/a%0d%0alevel=ERROR"
// cannot forge a log line and an oversized value cannot bloat the record.
func sanitizeLogValue(s string, maxLen int) string {
	s = corestrings.TruncateBytes(s, maxLen, TruncationSuffix)
	if !strings.ContainsFunc(s, unicode.IsControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// logCompletion logs the completed request: the context fields (which
// handlers may have modified), timing, correlation IDs and captured bodies.
func (m *middleware) logCompletion(ctx context.Context, status int, start time.Time, requestBody, responseBody string) {
	// Add timing fields
	now := time.Now()
	elapsed := now.Sub(start)

	// Get fields from context (may have been modified by handlers)
	fields := slogx.FieldsFromContext(ctx)
	fields = append(fields,
		slogx.Field{Key: FieldKeyHTTPStatus, Value: status},
		slogx.Field{Key: observability.FieldKeyRequestStartTime, Value: timeformat.FormatTime(start, m.opts.timeFormat)},
		slogx.Field{Key: observability.FieldKeyRequestEndTime, Value: timeformat.FormatTime(now, m.opts.timeFormat)},
		slogx.Field{Key: observability.FieldKeyRequestDuration, Value: timeformat.FormatDuration(elapsed, m.opts.timeFormat)},
	)
	fields = appendCorrelationFields(ctx, fields)

	// Add request body if captured
	if requestBody != "" {
		fields = append(fields, slogx.Field{Key: observability.FieldKeyRequestContent, Value: m.redactBody(requestBody)})
	}

	// Add response body if captured
	if responseBody != "" {
		fields = append(fields, slogx.Field{Key: observability.FieldKeyResponseContent, Value: m.redactBody(responseBody)})
	}

	m.logHandler.Log(ctx, "request completed", status, fields)
}

// appendCorrelationFields appends the request, trace and span IDs present
// in ctx.
func appendCorrelationFields(ctx context.Context, fields slogx.Fields) slogx.Fields {
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
	return fields
}

// redactBody applies the body redactor, if any, to a captured body.
func (m *middleware) redactBody(body string) string {
	if m.opts.bodyRedactor != nil {
		return m.opts.bodyRedactor(body)
	}
	return body
}

// New creates a new logger middleware with the given [LogHandler] and options.
// If logHandler is nil, the returned middleware is a no-op pass-through.
//
// This is the recommended way to create the logger middleware.
// For [slog.Logger], use: logger.New(logger.Slog(myLogger), opts...)
func New(logHandler LogHandler, opt ...Option) *middleware {
	if logHandler == nil {
		return &middleware{
			BaseMiddleware: middlewares.NewBaseMiddleware(middlewareName, nil),
		}
	}

	opts := newOptions(opt...)

	// Secure-by-default: body logging with no redactor would write raw request /
	// response bodies (credentials, tokens, PII) straight to logs. When body
	// logging is enabled without an explicit [WithBodyRedactor], install a
	// fully-masking redactor so nothing sensitive leaks. Callers that genuinely
	// want raw bodies opt in with an identity redactor:
	// WithBodyRedactor(func(b string) string { return b }).
	if (opts.logRequest || opts.logResponse) && opts.bodyRedactor == nil {
		opts.bodyRedactor = redactAllBody
	}

	// Build ignore methods map for O(1) lookup
	ignoreMethodsMap := make(map[string]struct{}, len(opts.ignoreMethods))
	for _, method := range opts.ignoreMethods {
		// Methods are already uppercased by optgen, but intern for consistency
		ignoreMethodsMap[intern.UpperString(method)] = struct{}{}
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

	redactQuery := make(map[string]struct{}, len(opts.redactQueryParams))
	for _, name := range opts.redactQueryParams {
		redactQuery[name] = struct{}{}
	}

	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			nil,
		),
		logHandler:       logHandler,
		opts:             opts,
		ignoreMethodsMap: ignoreMethodsMap,
		logCodesSet:      logCodesSet,
		ignoreCodesSet:   ignoreCodesSet,
		redactQuery:      coremaps.NewImmutableMap(redactQuery),
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

// captureBody reads up to [middlewares.MaxCaptureBodySize] bytes of body for
// logging and returns a replacement body that yields exactly what the handler
// would have read from the original: the captured prefix, then the unread
// remainder, or, when the capture read failed, the read error. Close always
// reaches the original body. The returned string is the captured prefix, or
// empty when the read failed.
func captureBody(body io.ReadCloser) (io.ReadCloser, string) {
	buf := coreio.GetBuffer()
	_, readErr := io.CopyN(buf, body, int64(middlewares.MaxCaptureBodySize))
	// Convert to string once (single allocation); strings.NewReader replays
	// it without another copy.
	prefix := buf.String()
	coreio.PutBuffer(buf)

	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readCloser{io.MultiReader(strings.NewReader(prefix), errReader{readErr}), body}, ""
	}
	return readCloser{io.MultiReader(strings.NewReader(prefix), body), body}, prefix
}

// readCloser pairs a replay reader with the original body's Close.
type readCloser struct {
	io.Reader
	io.Closer
}

// errReader replays an error consumed while capturing the body.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// redactedQuery returns raw with the value of every parameter listed in
// [WithRedactQueryParams] replaced by [RedactedQueryValue], keeping the
// parameters' order, separators and encoding. Both "&" and ";" separate
// parameters, since a handler behind [http.AllowQuerySemicolons] reads ";"
// as one. A name is compared after unescaping, so an encoded name
// ("acc%65ss_token") is redacted too.
func (m *middleware) redactedQuery(raw string) string {
	if m.redactQuery.Len() == 0 {
		return raw
	}
	var b strings.Builder
	b.Grow(len(raw))
	for raw != "" {
		part := raw
		sep := ""
		if i := strings.IndexAny(raw, "&;"); i >= 0 {
			part, sep, raw = raw[:i], raw[i:i+1], raw[i+1:]
		} else {
			raw = ""
		}
		key, _, _ := strings.Cut(part, "=")
		name, err := url.QueryUnescape(key)
		if err != nil {
			name = key
		}
		if m.redactQuery.Contains(name) {
			b.WriteString(key)
			b.WriteString("=" + RedactedQueryValue)
		} else {
			b.WriteString(part)
		}
		b.WriteString(sep)
	}
	return b.String()
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
