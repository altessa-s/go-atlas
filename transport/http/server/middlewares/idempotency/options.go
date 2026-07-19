// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"net/http"
	"regexp"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	internalidem "github.com/altessa-s/go-atlas/transport/internal/idempotency"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

const (
	// DefaultIdempotencyKeyHeader is the default HTTP header name for the idempotency key.
	DefaultIdempotencyKeyHeader = internalidem.DefaultKeyHeader
	// DefaultIdempotencyKeyStatusHeader is the default header name for idempotency status.
	DefaultIdempotencyKeyStatusHeader = internalidem.DefaultKeyStatusHeader
	// DefaultIdempotencyKeyEntityIdHeader is the default header name for entity ID.
	DefaultIdempotencyKeyEntityIdHeader = internalidem.DefaultKeyEntityIDHeader
)

// ErrorScenario identifies the reason for an idempotency error.
// Values of this type are passed to the configured [ErrorHandler] so it
// can produce an appropriate HTTP response for each scenario.
type ErrorScenario int

const (
	// ErrorIDKMissing indicates that the idempotency key is required but missing.
	ErrorIDKMissing ErrorScenario = iota
	// ErrorIDKInvalidFormat indicates that the idempotency key has invalid format (not lowercase UUID v4).
	ErrorIDKInvalidFormat
	// ErrorIDKInProgress indicates that a request with the same key is currently in progress.
	ErrorIDKInProgress
	// ErrorIDKAlreadyUsed indicates that the idempotency key has already been successfully used.
	ErrorIDKAlreadyUsed
)

// KeyFormatValidator validates the format of an idempotency key.
// Implementations must return nil if the key is valid, or [ErrInvalidFormat]
// if the format is invalid. The default validator is [DefaultKeyValidator],
// which requires a lowercase UUID v4. Override with [WithKeyFormatValidator].
type KeyFormatValidator func(key string) error

// ErrInvalidFormat is returned by [DefaultKeyValidator] (and any custom
// [KeyFormatValidator]) when the idempotency key is not a valid lowercase
// UUID v4. The middleware translates this into an [ErrorIDKInvalidFormat]
// scenario and invokes the configured [ErrorHandler].
var ErrInvalidFormat = internalidem.ErrInvalidFormat

// DefaultKeyValidator validates that the key is a valid lowercase UUID v4.
func DefaultKeyValidator(key string) error {
	return internalidem.DefaultKeyValidator(key)
}

// options holds configuration for the idempotency middleware.
type options struct {
	logger                       *slog.Logger
	ignorePaths                  []string
	ignorePatterns               []*regexp.Regexp   `optgen:"default=defaults.IgnorePatterns"`
	idempotencyKeyHeader         string             `optgen:"default=DefaultIdempotencyKeyHeader"`
	idempotencyKeyStatusHeader   string             `optgen:"default=DefaultIdempotencyKeyStatusHeader"`
	idempotencyKeyEntityIdHeader string             `optgen:"default=DefaultIdempotencyKeyEntityIdHeader"`
	fallbackBehavior             fallback.Behavior  `optgen:"default=fallback.Deny"`
	enforceMandatory             bool               `optgen:"default=false" optval:"param"`
	keyFormatValidator           KeyFormatValidator `optgen:"default=DefaultKeyValidator"`
	entityIdExtractor            EntityIdExtractor
	errorHandler                 ErrorHandler
}

// EntityIdExtractor extracts an entity identifier from a successful response.
// The middleware calls it after the downstream handler writes a 2xx status code,
// passing the request, the response Content-Type, and the captured body bytes.
// If the function returns a non-empty string, it is stored alongside the
// idempotency key and returned in the [DefaultIdempotencyKeyEntityIdHeader]
// header on duplicate requests. Configure with [WithEntityIdExtractor].
type EntityIdExtractor func(r *http.Request, contentType string, body []byte) string

// ErrorHandler writes an HTTP response for idempotency-related errors.
// The middleware invokes it with the [ErrorScenario] that triggered the error
// and, when available, the existing [idempotency.State] for the key.
// A default handler is provided; override it with [WithErrorHandler].
type ErrorHandler func(w http.ResponseWriter, r *http.Request, scenario ErrorScenario, state *idempotency.State)
