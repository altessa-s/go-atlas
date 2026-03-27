// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

const (
	// GrpcInterIdempotencyMaxKeyHeaderLength defines the maximum length for idempotency key header names.
	// Prevents excessively long header names that could cause HTTP/gRPC issues.
	GrpcInterIdempotencyMaxKeyHeaderLength = 128

	// GrpcInterIdempotencyMaxKeyFieldLength defines the maximum length for idempotency key field names.
	// Limits field name length for request parsing and proto field access.
	GrpcInterIdempotencyMaxKeyFieldLength = 100

	// Default values for GrpcInterIdempotencyConfig.
	defaultIdempotencyKeyHeader           = "idempotency-key"
	defaultIdempotencyKeyField            = "idempotencyKey"
	defaultIdempotencyKeyStatusMetadata   = "Idempotency-Key-Status"
	defaultIdempotencyKeyEntityIdMetadata = "Idempotency-Key-Entity-Id"
)

// GrpcInterIdempotencyConfig defines the configuration for idempotency interceptor.
// Controls how duplicate requests are detected and handled using configurable storage.
type GrpcInterIdempotencyConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`

	// IdempotencyKeyHeader is the header name for idempotency key.
	// Clients can provide idempotency keys via this header.
	// Must be a valid HTTP header name, defaults to "idempotency-key".
	IdempotencyKeyHeader string `yaml:"idempotencyKeyHeader" default:"idempotency-key"`

	// IdempotencyKeyField is the field name to extract idempotency key from request.
	// Used when idempotency key is embedded in the request message itself.
	// Must be a valid protobuf field name, defaults to "idempotencyKey".
	IdempotencyKeyField string `yaml:"idempotencyKeyField" default:"idempotencyKey"`

	// FallbackBehavior defines how the idempotency checker behaves when encountering errors
	// or when storage backends are unavailable, providing graceful degradation options.
	// See FallbackBehavior type for available options.
	FallbackBehavior FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// IdempotencyKeyStatusMetadata is the metadata key for idempotency status.
	IdempotencyKeyStatusMetadata string `yaml:"idempotencyKeyStatusMetadata" default:"Idempotency-Key-Status"`

	// IdempotencyKeyEntityIdMetadata is the metadata key for entity ID.
	IdempotencyKeyEntityIdMetadata string `yaml:"idempotencyKeyEntityIdMetadata" default:"Idempotency-Key-Entity-Id"`

	// EnforceMandatory controls whether the idempotency key is mandatory for all methods.
	EnforceMandatory bool `yaml:"enforceMandatory" default:"true"`
}

// IsEnabled returns true if idempotency checking is enabled.
// This is a convenience method to check if the interceptor should be active.
func (c *GrpcInterIdempotencyConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the idempotency interceptor configuration.
// Ensures all fields are properly configured and storage settings are valid.
//
// Validation rules:
//   - IdempotencyKeyHeader: must be non-empty and within length limits
//   - IdempotencyKeyField: must be non-empty and within length limits
//   - FallbackBehavior: must be a valid behavior
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//
// Returns an error if validation fails, nil otherwise.
func (c *GrpcInterIdempotencyConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.IdempotencyKeyHeader, validation.Required,
				validation.Length(1, GrpcInterIdempotencyMaxKeyHeaderLength)),
			validation.Field(&c.IdempotencyKeyField, validation.Required,
				validation.Length(1, GrpcInterIdempotencyMaxKeyFieldLength)),
			validation.Field(&c.FallbackBehavior, validation.Required,
				ozzo_rules.OneOf(FallbackBehaviorAllow, FallbackBehaviorDeny, FallbackBehaviorError)),
			validation.Field(&c.IdempotencyKeyStatusMetadata, validation.Required),
			validation.Field(&c.IdempotencyKeyEntityIdMetadata, validation.Required),
		)
	})
}

// DefaultGrpcInterIdempotencyConfig returns a GrpcInterIdempotencyConfig with default values.
// Idempotency checking is disabled by default.
func DefaultGrpcInterIdempotencyConfig() GrpcInterIdempotencyConfig {
	return GrpcInterIdempotencyConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
		IdempotencyKeyHeader:           defaultIdempotencyKeyHeader,
		IdempotencyKeyField:            defaultIdempotencyKeyField,
		FallbackBehavior:               FallbackBehaviorDeny,
		IdempotencyKeyStatusMetadata:   defaultIdempotencyKeyStatusMetadata,
		IdempotencyKeyEntityIdMetadata: defaultIdempotencyKeyEntityIdMetadata,
		EnforceMandatory:               true,
	}
}
