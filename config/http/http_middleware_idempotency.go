// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// IdempotencyKeyLogMode controls how the client-supplied idempotency key is
// rendered in the middleware's debug-level log records.
type IdempotencyKeyLogMode string

const (
	// IdempotencyKeyLogModeHashed logs a truncated SHA-256 digest of the key
	// instead of the key itself. This is the default.
	IdempotencyKeyLogModeHashed IdempotencyKeyLogMode = "hashed"

	// IdempotencyKeyLogModeFull logs the key verbatim. Keys are caller-chosen
	// and may embed user data; intended for development only.
	IdempotencyKeyLogModeFull IdempotencyKeyLogMode = "full"

	// IdempotencyKeyLogModeOff omits the key from log records entirely.
	IdempotencyKeyLogModeOff IdempotencyKeyLogMode = "off"
)

// AllIdempotencyKeyLogModes returns all valid IdempotencyKeyLogMode values.
func AllIdempotencyKeyLogModes() []IdempotencyKeyLogMode {
	return []IdempotencyKeyLogMode{
		IdempotencyKeyLogModeHashed,
		IdempotencyKeyLogModeFull,
		IdempotencyKeyLogModeOff,
	}
}

// IdempotencyMiddleware defines the configuration for HTTP idempotency middleware.
type IdempotencyMiddleware struct {
	// BaseMiddleware provides standard enable and filtering fields.
	BaseMiddleware `yaml:",inline"`

	// IdempotencyKeyHeader is the name of the HTTP header used to pass the idempotency key.
	// Defaults to "Idempotency-Key".
	IdempotencyKeyHeader string `yaml:"idempotencyKeyHeader" default:"Idempotency-Key"`

	// IdempotencyKeyStatusHeader is the name of the HTTP header used to return idempotency status.
	// Defaults to "Idempotency-Key-Status".
	IdempotencyKeyStatusHeader string `yaml:"idempotencyKeyStatusHeader" default:"Idempotency-Key-Status"`

	// IdempotencyKeyEntityIdHeader is the name of the HTTP header used to return an entity ID if captured.
	// Defaults to "Idempotency-Key-Entity-Id".
	IdempotencyKeyEntityIdHeader string `yaml:"idempotencyKeyEntityIDHeader" default:"Idempotency-Key-Entity-Id"`

	// FallbackBehavior defines how the idempotency checker behaves when encountering errors
	// or when storage backends are unavailable, providing graceful degradation options.
	FallbackBehavior middlewareconfig.FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// EnforceMandatory, when true, requires all requests (not ignored) to have an idempotency key.
	EnforceMandatory bool `yaml:"enforceMandatory" default:"false"`

	// KeyLogMode controls how the client-supplied idempotency key reaches
	// debug-level logs. Keys are caller-chosen and may embed user data, so
	// this defaults to "hashed" — a truncated SHA-256 digest that keeps
	// records correlated without carrying the raw value.
	KeyLogMode IdempotencyKeyLogMode `yaml:"keyLogMode" default:"hashed"`

	// DisableRequestFingerprint stops recording a fingerprint of the request
	// with each key. With it recorded (the default), a key reused for a
	// different request is refused instead of being treated as a replay.
	DisableRequestFingerprint bool `yaml:"disableRequestFingerprint" default:"false"`

	// ExposeEntityID echoes the stored entity id to a duplicate request.
	// Off by default: the caller presenting a duplicate key is not verified
	// to be the principal that created the entity, unless the Keeper checks
	// owners.
	ExposeEntityID bool `yaml:"exposeEntityID" default:"false"`

	// KeyQueryParam names a URL query parameter that carries the key
	// (such as request_id). A non-empty value takes
	// precedence over the header. Empty (the default) reads the header only.
	KeyQueryParam string `yaml:"keyQueryParam"`

	// KeyFormat selects the key validator: uuidv4 (lowercase UUID v4, the
	// default), opaque (visible ASCII up to MaxKeyLength) or pattern
	// (KeyPattern matching the whole key, up to MaxKeyLength).
	KeyFormat middlewareconfig.IdempotencyKeyFormat `yaml:"keyFormat" default:"uuidv4"`

	// KeyPattern is the regular expression of the pattern key format.
	KeyPattern string `yaml:"keyPattern"`

	// MaxKeyLength bounds opaque and pattern keys, in bytes; 0 is 128.
	MaxKeyLength int `yaml:"maxKeyLength"`

	// MandatoryPaths lists request paths that require a key, whatever
	// EnforceMandatory says; ignored paths and safe methods stay unchecked.
	MandatoryPaths []string `yaml:"mandatoryPaths"`

	// MandatoryPatterns lists regular expressions of request paths that
	// require a key, like MandatoryPaths.
	MandatoryPatterns []string `yaml:"mandatoryPatterns"`

	// ResponseReplay stores the status, ReplayHeaders and body of a completed
	// 2xx response and writes them to a repeat of the request instead of
	// 422. Off by default: the repeat's caller receives the
	// original response, so scope keys per principal.
	ResponseReplay bool `yaml:"responseReplay"`

	// MaxReplaySize bounds a stored body, in bytes; 0 is 256 KiB, the cap is
	// 1 MiB (the response capture limit). A larger body is not stored.
	MaxReplaySize int64 `yaml:"maxReplaySize"`

	// ReplayHeaders lists the response headers stored and replayed; empty
	// keeps the default (Content-Type, Content-Encoding, Content-Language,
	// Content-Disposition, Location, ETag, Last-Modified).
	ReplayHeaders []string `yaml:"replayHeaders"`
}

// Validate performs validation of the IdempotencyMiddleware.
func (c *IdempotencyMiddleware) Validate() error {
	return c.ValidateBase(func() error {
		rules := middlewareconfig.IdempotencyKeyRules(&c.KeyFormat, &c.KeyPattern, &c.MaxKeyLength, &c.MandatoryPaths, &c.MandatoryPatterns)
		return validationconfig.ValidateStruct(c, append(rules,
			validation.Field(&c.IdempotencyKeyHeader, validation.Required),
			validation.Field(&c.IdempotencyKeyStatusHeader, validation.Required),
			validation.Field(&c.IdempotencyKeyEntityIdHeader, validation.Required),
			validation.Field(&c.FallbackBehavior, validation.Required,
				middlewareconfig.FallbackBehaviorRule()),
			validation.Field(&c.KeyLogMode, validation.Required,
				ozzo_rules.OneOf(AllIdempotencyKeyLogModes()...)),
			validation.Field(&c.MaxReplaySize, validation.Min(int64(0))),
			validation.Field(&c.ReplayHeaders, validation.Each(validation.Required)),
		)...)
	})
}

// DefaultIdempotencyMiddleware returns a configuration for idempotency middleware with default values.
func DefaultIdempotencyMiddleware() IdempotencyMiddleware {
	return IdempotencyMiddleware{
		BaseMiddleware: BaseMiddleware{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		IdempotencyKeyHeader:         "Idempotency-Key",
		IdempotencyKeyStatusHeader:   "Idempotency-Key-Status",
		IdempotencyKeyEntityIdHeader: "Idempotency-Key-Entity-Id",
		FallbackBehavior:             middlewareconfig.FallbackBehaviorDeny,
		EnforceMandatory:             false,
		KeyLogMode:                   IdempotencyKeyLogModeHashed,
		KeyFormat:                    middlewareconfig.IdempotencyKeyFormatUUIDv4,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *IdempotencyMiddleware) IsEnabled() bool {
	return c != nil && c.Enabled
}
