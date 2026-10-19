// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/core/encoding/keys"
	"github.com/altessa-s/go-atlas/core/io/spool"
	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	internalidem "github.com/altessa-s/go-atlas/transport/internal/idempotency"
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
	// replayHeaders is the canonical set of opts.replayHeaders.
	replayHeaders *coremaps.ImmutableMap[string, struct{}]
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
	// A non-positive limit falls back to the default; the cap keeps the
	// spool's one-byte look-ahead from overflowing.
	if opts.fingerprintMaxBody <= 0 {
		opts.fingerprintMaxBody = DefaultFingerprintMaxBody
	}
	opts.fingerprintMaxBody = min(opts.fingerprintMaxBody, math.MaxInt64-1)
	if opts.fingerprintMemThreshold <= 0 {
		opts.fingerprintMemThreshold = DefaultFingerprintMemThreshold
	}
	// The stored body comes from the capture, which stops at its cap.
	if opts.maxReplaySize <= 0 {
		opts.maxReplaySize = DefaultMaxReplaySize
	}
	opts.maxReplaySize = min(opts.maxReplaySize, middlewares.MaxCaptureBodySize)
	headers := make(map[string]struct{}, len(opts.replayHeaders))
	for _, h := range opts.replayHeaders {
		headers[http.CanonicalHeaderKey(h)] = struct{}{}
	}

	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		i:             storage,
		opts:          opts,
		replayHeaders: coremaps.NewImmutableMap(headers),
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
		http.Error(w, "Idempotency key has an invalid format", http.StatusBadRequest)
	case ErrorIDKInProgress:
		http.Error(w, "A request with this idempotency key is currently being processed", http.StatusConflict)
	case ErrorIDKAlreadyUsed:
		http.Error(w, "This idempotency key has already been used", http.StatusUnprocessableEntity)
	case ErrorIDKMismatch:
		http.Error(w, "This idempotency key was used for a different request", http.StatusUnprocessableEntity)
	case ErrorIDKForbidden:
		http.Error(w, "This idempotency key belongs to another caller", http.StatusForbidden)
	case ErrorIDKBodyTooLarge:
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
	case ErrorIDKBodyUnreadable:
		http.Error(w, "Request body could not be read", http.StatusBadRequest)
	default:
		http.Error(w, "Unknown idempotency error", http.StatusInternalServerError)
	}
}

// Handler wraps an http.Handler with idempotency checking functionality.
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Check/Lock
		storageKey, lockState, err := m.checkIdempotency(w, r)
		// A fingerprinted body was spooled; release it once r is served.
		if sp, ok := r.Body.(*spool.Spool); ok {
			defer func() { _ = sp.Close() }() //nolint:errcheck // best-effort temp file removal
		}
		if err != nil {
			return
		}

		// 2. Wrap writer to capture status and body if needed
		sw := m.WrapResponseWriter(w, m.opts.entityIdExtractor != nil || m.opts.responseReplay)
		defer sw.Release()

		// 3. Execute
		next.ServeHTTP(sw, r)

		// 4. Lifecycle (Success/Error)
		if storageKey != "" {
			if sw.StatusCode() >= http.StatusOK && sw.StatusCode() < http.StatusMultipleChoices {
				data := m.completionData(r, sw)
				// Best-effort completion. ErrLockStolen here means the
				// lock TTL expired during processing and another node
				// took over — silently drop our result rather than
				// overwriting the new holder's state.
				_ = m.i.Complete(r.Context(), storageKey, data, lockState) //nolint:errcheck
			} else {
				// Release only the lock owned by this failed request.
				_ = m.i.Release(r.Context(), storageKey, lockState) //nolint:errcheck // Best-effort cleanup, main operation already failed
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

	policy := PolicyDefault
	if m.opts.requestClassifier != nil {
		policy = m.opts.requestClassifier(r)
	}
	if policy == PolicySkip {
		return "", nil, nil
	}

	idempotencyKey := m.requestKey(r)
	if idempotencyKey == "" {
		m.LogDebug(ctx, "no idempotency key found", path,
			slog.String("method", r.Method))

		if policy.Mandatory(m.opts.enforceMandatory) {
			m.opts.errorHandler(w, r, ErrorIDKMissing, nil)
			return "", nil, errIdempotencyKeyMissing
		}
		return "", nil, nil
	}

	if err := m.opts.keyFormatValidator(idempotencyKey); err != nil {
		m.LogDebug(ctx, "invalid idempotency key format", path,
			m.keyLogAttrs(ctx, r.Method, idempotencyKey)...)
		m.opts.errorHandler(w, r, ErrorIDKInvalidFormat, nil)
		return "", nil, errIdempotencyKeyInvalidFormat
	}

	storageKey := m.buildKey(r.Method, path, idempotencyKey)

	m.LogDebug(ctx, "checking idempotency key", path,
		m.keyLogAttrs(ctx, r.Method, idempotencyKey)...)

	fingerprint, err := m.requestFingerprint(r)
	if err != nil {
		scenario := ErrorIDKBodyUnreadable
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge || errors.Is(err, spool.ErrTooLarge) {
			scenario = ErrorIDKBodyTooLarge
		}
		m.LogDebug(ctx, "request body could not be fingerprinted", path, slog.Any("error", err))
		m.opts.errorHandler(w, r, scenario, nil)
		return "", nil, errKeyNotUnique
	}

	// Try to acquire lock
	locked, state, err := m.i.AttemptLockWithOpts(ctx, storageKey, idempotency.AttemptLockOpts{
		Fingerprint: fingerprint,
	})
	outcome, record := internalidem.ClassifyLock(locked, state, err)
	switch outcome {
	case internalidem.LockFingerprintMismatch:
		m.LogDebug(ctx, "idempotency key used for a different request", path, m.keyLogAttrs(ctx, r.Method, idempotencyKey)...)
		m.opts.errorHandler(w, r, ErrorIDKMismatch, nil)
		return "", nil, errKeyNotUnique
	case internalidem.LockOwnerMismatch:
		m.LogDebug(ctx, "idempotency key belongs to another caller", path, m.keyLogAttrs(ctx, r.Method, idempotencyKey)...)
		// A ReplayAuthorizer refusal carrying an HTTP status (404 to hide
		// the record) is answered with that status and its standard text.
		if se, ok := errors.AsType[httpStatusError](err); ok {
			if code := se.HTTPStatus(); code >= http.StatusBadRequest && code <= 599 {
				http.Error(w, http.StatusText(code), code)
				return "", nil, errKeyNotUnique
			}
		}
		m.opts.errorHandler(w, r, ErrorIDKForbidden, nil)
		return "", nil, errKeyNotUnique
	case internalidem.LockStorageError:
		return "", nil, m.handleStorageError(w, r, err)
	case internalidem.LockNilState:
		return "", nil, m.handleStorageError(w, r, errors.New("lock failed but state is nil"))
	case internalidem.LockInProgress:
		m.LogDebug(ctx, "idempotency key in progress", path,
			m.keyLogAttrs(ctx, r.Method, idempotencyKey)...)

		w.Header().Set(m.opts.idempotencyKeyStatusHeader, "in_progress")
		m.opts.errorHandler(w, r, ErrorIDKInProgress, state)
		return "", nil, errKeyNotUnique
	case internalidem.LockCompleted:
		m.LogDebug(ctx, "idempotency key already used", path,
			m.keyLogAttrs(ctx, r.Method, idempotencyKey)...)

		w.Header().Set(m.opts.idempotencyKeyStatusHeader, "success")
		if m.opts.exposeEntityID && record.EntityID != "" {
			w.Header().Set(m.opts.idempotencyKeyEntityIdHeader, record.EntityID)
		}
		if m.opts.responseReplay && m.replay(w, record, replayBinding(r)) {
			return "", nil, errKeyNotUnique
		}
		m.opts.errorHandler(w, r, ErrorIDKAlreadyUsed, state)
		return "", nil, errKeyNotUnique
	case internalidem.LockAcquired:
	}

	m.LogDebug(ctx, "idempotency key registered", path,
		m.keyLogAttrs(ctx, r.Method, idempotencyKey)...)

	// Return the *State so Handler can pass its CAS token to Complete.
	return storageKey, state, nil
}

// completionData returns the data stored with a completed key: without
// [WithResponseReplay] the entity ID string (or nil), as before replay
// existed; with it an internalidem.Record holding the entity ID and, when the
// whole body was captured and is no larger than maxReplaySize, the status,
// the replay headers and the body (otherwise no response, so a repeat is
// refused as already used).
func (m *middleware) completionData(r *http.Request, sw *middlewares.ResponseWriter) any {
	var entityID string
	if m.opts.entityIdExtractor != nil {
		entityID = m.opts.entityIdExtractor(r, sw.Header().Get("Content-Type"), sw.Body())
	}
	if !m.opts.responseReplay {
		return internalidem.EntityIDData(entityID)
	}
	record := internalidem.NewRecord(entityID, replayBinding(r))
	body := sw.Body()
	if sw.BytesWritten() == int64(len(body)) && int64(len(body)) <= m.opts.maxReplaySize {
		record.Status = sw.StatusCode()
		record.Header = m.filterReplayHeaders(sw.Header())
		record.Body = bytes.Clone(body)
	}
	return record
}

// filterReplayHeaders returns the replay headers of h, nil when none.
func (m *middleware) filterReplayHeaders(h http.Header) map[string][]string {
	var out map[string][]string
	for name, values := range h {
		if !m.replayHeaders.Contains(http.CanonicalHeaderKey(name)) || len(values) == 0 {
			continue
		}
		if out == nil {
			out = make(map[string][]string)
		}
		out[http.CanonicalHeaderKey(name)] = slices.Clone(values)
	}
	return out
}

// replayBinding names the operation of r a stored response belongs to: its
// method and path.
func replayBinding(r *http.Request) string { return r.Method + " " + r.URL.Path }

// replay writes the response stored in record — its replay headers, status
// and body — and reports whether it did. A record without a stored 2xx
// response for binding (another method or path, or written before records
// carried a binding) writes nothing.
func (m *middleware) replay(w http.ResponseWriter, record internalidem.Record, binding string) bool {
	if !record.HasHTTPResponse(binding) || record.Status < http.StatusOK || record.Status >= http.StatusMultipleChoices {
		return false
	}
	maps.Copy(w.Header(), m.filterReplayHeaders(record.Header))
	w.WriteHeader(record.Status)
	_, _ = w.Write(record.Body) //nolint:errcheck // the client may be gone; nothing to do
	return true
}

// requestKey returns the idempotency key of r: the keyQueryParam query
// parameter when it holds a non-empty value, otherwise the key header.
func (m *middleware) requestKey(r *http.Request) string {
	if m.opts.keyQueryParam != "" {
		if key := strings.TrimSpace(r.URL.Query().Get(m.opts.keyQueryParam)); key != "" {
			return key
		}
	}
	return strings.TrimSpace(r.Header.Get(m.opts.idempotencyKeyHeader))
}

// httpStatusError is an error carrying an HTTP status, as
// writer.HTTPStatuser errors do.
type httpStatusError interface {
	error
	HTTPStatus() int
}

// requestFingerprint returns the fingerprint of r's method, path, query and
// whole body, or nil when fingerprinting is off. The body is hashed while it
// is spooled — in memory up to a threshold, then in a temp file — and r.Body
// is replaced with the spool (a *spool.Spool, which Handler closes), so the
// handler reads exactly what the client sent. A body over
// fingerprintMaxBody fails with [spool.ErrTooLarge], one that fails to read
// with its read error; the request must then be refused, since part of its
// body has been consumed.
func (m *middleware) requestFingerprint(r *http.Request) ([]byte, error) {
	if !m.opts.requestFingerprint {
		return nil, nil
	}
	bodyHash := sha256.New()
	if r.Body != nil && r.Body != http.NoBody {
		sp, err := spool.New(r.Body,
			spool.WithMaxBytes(m.opts.fingerprintMaxBody),
			spool.WithMemThreshold(min(m.opts.fingerprintMaxBody, m.opts.fingerprintMemThreshold)),
			spool.WithTee(bodyHash),
			spool.WithTempDir(m.opts.fingerprintTempDir),
		)
		// The spool read what it needs and the request now reads from the
		// spool: close the original, so a wrapper (a decompressor, a file)
		// releases its resources.
		closeErr := r.Body.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			_ = sp.Close() //nolint:errcheck // already failing
			return nil, closeErr
		}
		r.Body = sp
	}
	return keys.Digest([]byte(r.Method), []byte(r.URL.Path), []byte(r.URL.RawQuery), bodyHash.Sum(nil)), nil
}

// keyLogHashLength is the number of hex characters of the SHA-256 digest kept
// when rendering an idempotency key under [KeyLogHashed]. 64 bits is enough to
// correlate records for the same key without carrying the full digest.
const keyLogHashLength = 16

// keyLogAttrs builds the debug-log attributes for a request, rendering the
// client-supplied idempotency key according to the configured [KeyLogMode].
// An unset or unrecognized mode renders as [KeyLogHashed].
//
// It returns nil when debug logging is disabled, so the digest is not computed
// on the request path of a production logger running at Info or above.
//
// Hashing is log hygiene, not a privacy guarantee: a key drawn from a small or
// guessable set can still be recovered by hashing candidates. It keeps raw
// caller-controlled bytes — which under [ErrorIDKInvalidFormat] have not
// passed any validation — out of the log.
func (m *middleware) keyLogAttrs(ctx context.Context, method, key string) []slog.Attr {
	if !m.Logger().Enabled(ctx, slog.LevelDebug) {
		return nil
	}

	switch m.opts.keyLogMode {
	case KeyLogFull:
		return []slog.Attr{slog.String("method", method), slog.String("key", key)}
	case KeyLogOff:
		return []slog.Attr{slog.String("method", method)}
	default:
		return []slog.Attr{slog.String("method", method), slog.String("key_hash", hashKeyForLog(key))}
	}
}

// hashKeyForLog returns the truncated SHA-256 hex digest of key used by
// [KeyLogHashed].
func hashKeyForLog(key string) string {
	return corehash.SHA256HexString(key)[:keyLogHashLength]
}

// errKeyNotUnique is an internal error indicating the key already exists.
var errKeyNotUnique = http.ErrAbortHandler

// errIdempotencyKeyMissing is an internal error indicating the key is missing.
var errIdempotencyKeyMissing = http.ErrAbortHandler

// errIdempotencyKeyInvalidFormat is an internal error indicating the key has invalid format.
var errIdempotencyKeyInvalidFormat = http.ErrAbortHandler

// buildKey constructs a storage key from HTTP method, path, and idempotency key
// with internalidem.BuildStorageKey: the tuple ("transport", service, key) in the
// length-prefixed v2 format. For HTTP, service is represented as {method}:/{path}.
// Example: idk:v2:9:transport18:POST:/api/v1/users36:550e8400-e29b-41d4-a716-446655440000
func (m *middleware) buildKey(method, path, idempotencyKey string) string {
	path = corestrings.InternString(strings.TrimPrefix(path, "/"))
	return internalidem.BuildStorageKey(method+":/"+path, idempotencyKey)
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
