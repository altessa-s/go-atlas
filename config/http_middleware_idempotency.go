// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
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

// HttpInterIdempotencyConfig defines the configuration for HTTP idempotency middleware.
type HttpInterIdempotencyConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// IdempotencyKeyHeader is the name of the HTTP header used to pass the idempotency key.
	// Defaults to "Idempotency-Key".
	IdempotencyKeyHeader string `yaml:"idempotencyKeyHeader" default:"Idempotency-Key"`

	// IdempotencyKeyStatusHeader is the name of the HTTP header used to return idempotency status.
	// Defaults to "Idempotency-Key-Status".
	IdempotencyKeyStatusHeader string `yaml:"idempotencyKeyStatusHeader" default:"Idempotency-Key-Status"`

	// IdempotencyKeyEntityIdHeader is the name of the HTTP header used to return an entity ID if captured.
	// Defaults to "Idempotency-Key-Entity-Id".
	IdempotencyKeyEntityIdHeader string `yaml:"idempotencyKeyEntityIdHeader" default:"Idempotency-Key-Entity-Id"`

	// FallbackBehavior defines how the idempotency checker behaves when encountering errors
	// or when storage backends are unavailable, providing graceful degradation options.
	FallbackBehavior FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// EnforceMandatory, when true, requires all requests (not ignored) to have an idempotency key.
	EnforceMandatory bool `yaml:"enforceMandatory" default:"false"`

	// KeyLogMode controls how the client-supplied idempotency key reaches
	// debug-level logs. Keys are caller-chosen and may embed user data, so
	// this defaults to "hashed" — a truncated SHA-256 digest that keeps
	// records correlated without carrying the raw value.
	KeyLogMode IdempotencyKeyLogMode `yaml:"keyLogMode" default:"hashed"`
}

// Validate performs validation of the HttpInterIdempotencyConfig.
func (c *HttpInterIdempotencyConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.IdempotencyKeyHeader, validation.Required),
			validation.Field(&c.IdempotencyKeyStatusHeader, validation.Required),
			validation.Field(&c.IdempotencyKeyEntityIdHeader, validation.Required),
			validation.Field(&c.FallbackBehavior, validation.Required,
				ozzo_rules.OneOf(FallbackBehaviorAllow, FallbackBehaviorDeny, FallbackBehaviorError)),
			validation.Field(&c.KeyLogMode, validation.Required,
				ozzo_rules.OneOf(AllIdempotencyKeyLogModes()...)),
		)
	})
}

// DefaultHttpInterIdempotencyConfig returns a configuration for idempotency middleware with default values.
func DefaultHttpInterIdempotencyConfig() HttpInterIdempotencyConfig {
	return HttpInterIdempotencyConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
		IdempotencyKeyHeader:         "Idempotency-Key",
		IdempotencyKeyStatusHeader:   "Idempotency-Key-Status",
		IdempotencyKeyEntityIdHeader: "Idempotency-Key-Entity-Id",
		FallbackBehavior:             FallbackBehaviorDeny,
		EnforceMandatory:             false,
		KeyLogMode:                   IdempotencyKeyLogModeHashed,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *HttpInterIdempotencyConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}
