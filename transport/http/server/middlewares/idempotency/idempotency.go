// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// HTTP status codes used by the middleware.
const (
	StatusConflict            = http.StatusConflict
	StatusServiceUnavailable  = http.StatusServiceUnavailable
	StatusInternalServerError = http.StatusInternalServerError
)

// Response messages.
const (
	messageServiceTemporaryUnavailable = "Service Temporarily Unavailable"
	messageInternalServerError         = "Internal Server Error"
)

const middlewareName = "idempotency"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

// Idempotency is an alias for [idempotency.Idempotency] from data/idempotency.
type Idempotency = idempotency.Idempotency

type middleware struct {
	middlewares.BaseMiddleware
	i    Idempotency
	opts *options
}

// Dependencies returns middlewares that idempotency requires to run before it.
// Idempotency has no dependencies.
func (m *middleware) Dependencies() []string {
	return nil
}

// New creates a new idempotency middleware with the given
// [Idempotency] storage backend and [Option] values.
// If no [WithErrorHandler] option is provided, a default handler that
// writes plain-text error responses is used.
func New(storage Idempotency, opt ...Option) *middleware {
	opts := newOptions(opt...)

	if opts.errorHandler == nil {
		opts.errorHandler = defaultErrorHandler
	}

	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		i:    storage,
		opts: opts,
	}
}

// Middleware returns an HTTP middleware that prevents duplicate request processing.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func Middleware(storage Idempotency, opt ...Option) func(http.Handler) http.Handler {
	return New(storage, opt...).Handler
}

// defaultErrorHandler handles idempotency errors by writing a plain text response.
// Override with [WithErrorHandler] to produce structured (JSON/XML) responses.
func defaultErrorHandler(w http.ResponseWriter, _ *http.Request, scenario ErrorScenario, _ *idempotency.State) {
	switch scenario {
	case ErrorIDKMissing:
		http.Error(w, "Idempotency key is required for this operation", http.StatusBadRequest)
	case ErrorIDKInvalidFormat:
		http.Error(w, "Idempotency key must be a valid lowercase UUID v4", http.StatusBadRequest)
	case ErrorIDKInProgress:
		http.Error(w, "A request with this idempotency key is currently being processed", http.StatusConflict)
	case ErrorIDKAlreadyUsed:
		http.Error(w, "This idempotency key has already been used", http.StatusUnprocessableEntity)
	default:
		http.Error(w, "Unknown idempotency error", http.StatusInternalServerError)
	}
}

// Handler wraps an http.Handler with idempotency checking functionality.
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Check/Lock
		storageKey, lockState, err := m.checkIdempotency(w, r)
		if err != nil {
			return
		}

		// 2. Wrap writer to capture status and body if needed
		sw := m.WrapResponseWriter(w, m.opts.entityIdExtractor != nil)
		defer sw.Release()

		// 3. Execute
		next.ServeHTTP(sw, r)

		// 4. Lifecycle (Success/Error)
		if storageKey != "" {
			if sw.StatusCode() >= http.StatusOK && sw.StatusCode() < http.StatusMultipleChoices {
				var data any
				if m.opts.entityIdExtractor != nil {
					contentType := sw.Header().Get("Content-Type")
					if entityID := m.opts.entityIdExtractor(r, contentType, sw.Body()); entityID != "" {
						data = entityID
					}
				}
				// Best-effort completion. ErrLockStolen here means the
				// lock TTL expired during processing and another node
				// took over — silently drop our result rather than
				// overwriting the new holder's state.
				_ = m.i.Complete(r.Context(), storageKey, data, lockState) //nolint:errcheck
			} else {
				// On error, delete the key
				_ = m.i.Delete(r.Context(), storageKey) //nolint:errcheck // Best-effort cleanup, main operation already failed
			}
		}
	})
}

func (m *middleware) checkIdempotency(w http.ResponseWriter, r *http.Request) (string, *idempotency.State, error) {
	path := m.InternPath(r.URL.Path)
	ctx := r.Context()

	if m.ShouldIgnore(path) {
		m.LogIgnored(ctx, path)
		return "", nil, nil
	}

	// Safe HTTP methods are idempotent by definition — skip check.
	if isSafeMethod(r.Method) {
		return "", nil, nil
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get(m.opts.idempotencyKeyHeader))
	if idempotencyKey == "" {
		m.LogDebug(ctx, "no idempotency key found", path,
			slog.String("method", r.Method))

		if m.opts.enforceMandatory {
			m.opts.errorHandler(w, r, ErrorIDKMissing, nil)
			return "", nil, errIdempotencyKeyMissing
		}
		return "", nil, nil
	}

	if err := m.opts.keyFormatValidator(idempotencyKey); err != nil {
		m.LogDebug(ctx, "invalid idempotency key format", path,
			slog.String("method", r.Method),
			slog.String("key", idempotencyKey))
		m.opts.errorHandler(w, r, ErrorIDKInvalidFormat, nil)
		return "", nil, errIdempotencyKeyInvalidFormat
	}

	storageKey := m.buildKey(r.Method, path, idempotencyKey)

	m.LogDebug(ctx, "checking idempotency key", path,
		slog.String("method", r.Method),
		slog.String("key", idempotencyKey))

	// Try to acquire lock
	locked, state, err := m.i.AttemptLock(ctx, storageKey)
	if err != nil {
		return "", nil, m.handleStorageError(w, r, err)
	}

	if !locked {
		if state == nil {
			return "", nil, m.handleStorageError(w, r, errors.New("lock failed but state is nil"))
		}

		if state.Status == idempotency.StatusInProgress {
			m.LogDebug(ctx, "idempotency key in progress", path,
				slog.String("method", r.Method),
				slog.String("key", idempotencyKey))

			w.Header().Set(m.opts.idempotencyKeyStatusHeader, "in_progress")
			m.opts.errorHandler(w, r, ErrorIDKInProgress, state)
			return "", nil, errKeyNotUnique
		}

		if state.Status == idempotency.StatusSuccess {
			m.LogDebug(ctx, "idempotency key already used", path,
				slog.String("method", r.Method),
				slog.String("key", idempotencyKey))

			w.Header().Set(m.opts.idempotencyKeyStatusHeader, "success")
			if strVal, ok := state.Data.(string); ok && strVal != "" {
				w.Header().Set(m.opts.idempotencyKeyEntityIdHeader, strVal)
			}
			m.opts.errorHandler(w, r, ErrorIDKAlreadyUsed, state)
			return "", nil, errKeyNotUnique
		}
	}

	m.LogDebug(ctx, "idempotency key registered", path,
		slog.String("method", r.Method),
		slog.String("key", idempotencyKey))

	// Return the *State so Handler can pass its CAS token to Complete.
	return storageKey, state, nil
}

// errKeyNotUnique is an internal error indicating the key already exists.
var errKeyNotUnique = http.ErrAbortHandler

// errIdempotencyKeyMissing is an internal error indicating the key is missing.
var errIdempotencyKeyMissing = http.ErrAbortHandler

// errIdempotencyKeyInvalidFormat is an internal error indicating the key has invalid format.
var errIdempotencyKeyInvalidFormat = http.ErrAbortHandler

// buildKey constructs a storage key from HTTP method, path, and idempotency key.
// Format idk:{service}:{idempotency_key}
// For HTTP, service is represented as {method}:{path}.
// Example: idk:POST:/api/v1/users:550e8400-e29b-41d4-a716-446655440000
func (m *middleware) buildKey(method, path, idempotencyKey string) string {
	path = corestrings.InternString(strings.TrimPrefix(path, "/"))
	return "idk:" + method + ":/" + path + ":" + idempotencyKey
}

// isSafeMethod reports whether the HTTP method is safe (idempotent by definition).
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func (m *middleware) handleStorageError(w http.ResponseWriter, r *http.Request, err error) error {
	m.LogWarn(r.Context(), "idempotency storage error", r.URL.Path, err,
		slog.String("method", r.Method))

	switch m.opts.fallbackBehavior {
	case fallback.Allow:
		return nil
	case fallback.Deny:
		http.Error(w, messageServiceTemporaryUnavailable, StatusServiceUnavailable)
		return err
	default:
		http.Error(w, messageInternalServerError, StatusInternalServerError)
		return err
	}
}
