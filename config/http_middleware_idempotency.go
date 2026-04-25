// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

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
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *HttpInterIdempotencyConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}
