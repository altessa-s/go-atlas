// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/validation"

	"google.golang.org/grpc/status"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

const (
	// DefaultIdempotencyKeyHeader is the default header name for idempotency key.
	// Per ADR-26-01-12-002, the header name is "Idempotency-Key".
	DefaultIdempotencyKeyHeader = "Idempotency-Key"
	// DefaultIdempotencyKeyStatusMetadata is the default metadata key for idempotency status.
	DefaultIdempotencyKeyStatusMetadata = "Idempotency-Key-Status"
	// DefaultIdempotencyKeyEntityIdMetadata is the default metadata key for entity ID.
	DefaultIdempotencyKeyEntityIdMetadata = "Idempotency-Key-Entity-Id"
)

// ErrorScenario identifies the reason for an idempotency error.
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
// Returns nil if the key is valid, or ErrInvalidFormat if invalid.
type KeyFormatValidator func(key string) error

// ErrInvalidFormat is returned when the idempotency key format is invalid.
var ErrInvalidFormat = errors.New("invalid idempotency key format")

// DefaultKeyValidator validates that the key is a valid lowercase UUID v4.
func DefaultKeyValidator(key string) error {
	if strings.ToLower(key) != key || !validation.IsValidUUIDv4(key) {
		return ErrInvalidFormat
	}
	return nil
}

// options holds configuration for the idempotency interceptor.
type options struct {
	ignoreMethods                  []string
	ignorePatterns                 []*regexp.Regexp   `optgen:"default=defaults.IgnorePatterns"`
	idempotencyKeyHeader           string             `optgen:"default=DefaultIdempotencyKeyHeader"`
	idempotencyKeyStatusMetadata   string             `optgen:"default=DefaultIdempotencyKeyStatusMetadata"`
	idempotencyKeyEntityIdMetadata string             `optgen:"default=DefaultIdempotencyKeyEntityIdMetadata"`
	fallbackBehavior               fallback.Behavior  `optgen:"default=fallback.Deny"`
	enforceMandatory               bool               `optgen:"default=false" optval:"param"`
	keyFormatValidator             KeyFormatValidator `optgen:"default=DefaultKeyValidator"`
	entityIdExtractor              EntityIdExtractor
	statusCreator                  StatusCreator
}

// EntityIdExtractor serves for extracting entity ID from the response.
type EntityIdExtractor func(method string, resp any) string

// StatusCreator serves for creating a custom gRPC status error for idempotency scenarios.
type StatusCreator func(ctx context.Context, scenario ErrorScenario, state *idempotency.State) *status.Status
