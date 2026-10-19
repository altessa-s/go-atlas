// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig

import (
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

const (
	// IdempotencyMaxKeyHeaderLength defines the maximum length for idempotency key header names.
	// Prevents excessively long header names that could cause HTTP/gRPC issues.
	IdempotencyMaxKeyHeaderLength = 128

	// IdempotencyMaxKeyFieldLength defines the maximum length for idempotency key field names.
	// Limits field name length for request parsing and proto field access.
	IdempotencyMaxKeyFieldLength = 100

	// Default values for IdempotencyInterceptor.
	defaultIdempotencyKeyHeader           = "idempotency-key"
	defaultIdempotencyKeyStatusMetadata   = "Idempotency-Key-Status"
	defaultIdempotencyKeyEntityIdMetadata = "Idempotency-Key-Entity-Id"
)

// IdempotencyInterceptor defines the configuration for idempotency interceptor.
// Controls how duplicate requests are detected and handled using configurable storage.
type IdempotencyInterceptor struct {
	// BaseInterceptor provides common interceptor configuration.
	BaseInterceptor `yaml:",inline"`

	// IdempotencyKeyHeader is the header name for idempotency key.
	// Clients can provide idempotency keys via this header.
	// Must be a valid HTTP header name, defaults to "idempotency-key".
	IdempotencyKeyHeader string `yaml:"idempotencyKeyHeader" default:"idempotency-key"`

	// IdempotencyKeyField names a string field of the request message that
	// carries the key (such as request_id), by proto or JSON name. A
	// non-empty value in the field takes precedence over the header. Empty
	// (the default) reads the header only.
	IdempotencyKeyField string `yaml:"idempotencyKeyField"`

	// KeyFormat selects the key validator: uuidv4 (lowercase UUID v4, the
	// default), opaque (visible ASCII up to MaxKeyLength) or pattern
	// (KeyPattern matching the whole key, up to MaxKeyLength).
	KeyFormat middlewareconfig.IdempotencyKeyFormat `yaml:"keyFormat" default:"uuidv4"`

	// KeyPattern is the regular expression of the pattern key format.
	KeyPattern string `yaml:"keyPattern"`

	// MaxKeyLength bounds opaque and pattern keys, in bytes; 0 is 128.
	MaxKeyLength int `yaml:"maxKeyLength"`

	// MethodClassifier selects which methods take part: "" (every method
	// not ignored) or "verbs" (methods are classified by the verb their
	// name starts with: Create, BatchCreate and custom verbs take part; Get,
	// List, Search, Update, Delete and their Batch forms are skipped).
	MethodClassifier IdempotencyMethodClassifier `yaml:"methodClassifier"`

	// MandatoryMethods lists fully qualified methods that require a key,
	// whatever EnforceMandatory says; ignored methods stay ignored.
	MandatoryMethods []string `yaml:"mandatoryMethods"`

	// MandatoryPatterns lists regular expressions of methods that require a
	// key, like MandatoryMethods.
	MandatoryPatterns []string `yaml:"mandatoryPatterns"`

	// ResponseReplay stores the response of a completed unary call and
	// returns it to a repeat of the request instead of
	// FAILED_PRECONDITION. Off by default: the repeat's caller receives the
	// original response, so scope keys per principal.
	ResponseReplay bool `yaml:"responseReplay"`

	// MaxReplaySize bounds a stored response, in bytes of the marshaled
	// message; 0 is 256 KiB. A larger response is not stored.
	MaxReplaySize int `yaml:"maxReplaySize"`

	// FallbackBehavior defines how the idempotency checker behaves when encountering errors
	// or when storage backends are unavailable, providing graceful degradation options.
	// See FallbackBehavior type for available options.
	FallbackBehavior middlewareconfig.FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// IdempotencyKeyStatusMetadata is the metadata key for idempotency status.
	IdempotencyKeyStatusMetadata string `yaml:"idempotencyKeyStatusMetadata" default:"Idempotency-Key-Status"`

	// IdempotencyKeyEntityIdMetadata is the metadata key for entity ID.
	IdempotencyKeyEntityIdMetadata string `yaml:"idempotencyKeyEntityIDMetadata" default:"Idempotency-Key-Entity-Id"`

	// EnforceMandatory controls whether the idempotency key is mandatory for all methods.
	EnforceMandatory bool `yaml:"enforceMandatory" default:"false"`

	// DisableRequestFingerprint stops recording a fingerprint of the request
	// with each key. With it recorded (the default), a key reused for a
	// different request is refused instead of being treated as a replay.
	DisableRequestFingerprint bool `yaml:"disableRequestFingerprint" default:"false"`

	// ExposeEntityID echoes the stored entity id to a duplicate request.
	// Off by default: the caller presenting a duplicate key is not verified
	// to be the principal that created the entity, unless the Keeper checks
	// owners.
	ExposeEntityID bool `yaml:"exposeEntityID" default:"false"`
}

// IdempotencyMethodClassifier selects the methods the idempotency interceptor
// checks.
type IdempotencyMethodClassifier string

const (
	// IdempotencyMethodClassifierAll checks every method not ignored.
	IdempotencyMethodClassifierAll IdempotencyMethodClassifier = ""
	// IdempotencyMethodClassifierVerbs classifies methods by the verb their
	// name starts with: Create, BatchCreate and custom verbs are checked;
	// Get, List, Search, Update, Delete and their Batch forms are skipped.
	IdempotencyMethodClassifierVerbs IdempotencyMethodClassifier = "verbs"
)

// IsEnabled returns true if idempotency checking is enabled.
// This is a convenience method to check if the interceptor should be active.
func (c *IdempotencyInterceptor) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the idempotency interceptor configuration.
// Ensures all fields are properly configured and storage settings are valid.
//
// Validation rules:
//   - IdempotencyKeyHeader: must be non-empty and within length limits
//   - IdempotencyKeyField: within length limits when set
//   - KeyFormat: a known format; KeyPattern required (and compiling) for pattern
//   - MaxKeyLength: not negative
//   - MethodClassifier: "" or "verbs"; MandatoryPatterns must compile
//   - FallbackBehavior: must be a valid behavior
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//
// Returns an error if validation fails, nil otherwise.
func (c *IdempotencyInterceptor) Validate() error {
	return c.ValidateBase(func() error {
		rules := middlewareconfig.IdempotencyKeyRules(&c.KeyFormat, &c.KeyPattern, &c.MaxKeyLength, &c.MandatoryMethods, &c.MandatoryPatterns)
		return validationconfig.ValidateStruct(c, append(rules,
			validation.Field(&c.IdempotencyKeyHeader, validation.Required,
				validation.Length(1, IdempotencyMaxKeyHeaderLength)),
			validation.Field(&c.IdempotencyKeyField, validation.Length(0, IdempotencyMaxKeyFieldLength)),
			validation.Field(&c.MethodClassifier, validation.In(IdempotencyMethodClassifierAll, IdempotencyMethodClassifierVerbs)),
			validation.Field(&c.MaxReplaySize, validation.Min(0)),
			validation.Field(&c.FallbackBehavior, validation.Required,
				middlewareconfig.FallbackBehaviorRule()),
			validation.Field(&c.IdempotencyKeyStatusMetadata, validation.Required),
			validation.Field(&c.IdempotencyKeyEntityIdMetadata, validation.Required),
		)...)
	})
}

// DefaultIdempotencyInterceptor returns a IdempotencyInterceptor with default values.
// Idempotency checking is disabled by default.
func DefaultIdempotencyInterceptor() IdempotencyInterceptor {
	return IdempotencyInterceptor{
		BaseInterceptor: BaseInterceptor{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		IdempotencyKeyHeader:           defaultIdempotencyKeyHeader,
		KeyFormat:                      middlewareconfig.IdempotencyKeyFormatUUIDv4,
		FallbackBehavior:               middlewareconfig.FallbackBehaviorDeny,
		IdempotencyKeyStatusMetadata:   defaultIdempotencyKeyStatusMetadata,
		IdempotencyKeyEntityIdMetadata: defaultIdempotencyKeyEntityIdMetadata,
		EnforceMandatory:               false,
	}
}
