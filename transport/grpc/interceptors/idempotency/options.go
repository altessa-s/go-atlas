// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"context"
	"regexp"
	"slices"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	"google.golang.org/grpc/status"

	internalidem "github.com/altessa-s/go-atlas/transport/internal/idempotency"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

const (
	// DefaultIdempotencyKeyHeader is the default header name for idempotency key.
	// Per ADR-26-01-12-002, the header name is "Idempotency-Key".
	DefaultIdempotencyKeyHeader = internalidem.DefaultKeyHeader
	// DefaultIdempotencyKeyStatusMetadata is the default metadata key for idempotency status.
	DefaultIdempotencyKeyStatusMetadata = internalidem.DefaultKeyStatusHeader
	// DefaultIdempotencyKeyEntityIdMetadata is the default metadata key for entity ID.
	DefaultIdempotencyKeyEntityIdMetadata = internalidem.DefaultKeyEntityIDHeader
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
	// ErrorIDKMismatch indicates that the idempotency key was used for a
	// different request (see [WithRequestFingerprint]).
	ErrorIDKMismatch
	// ErrorIDKForbidden indicates that the idempotency key belongs to another
	// caller (see idempotency.WithOwner).
	ErrorIDKForbidden
)

// Reason codes attached to the [errdetails.ErrorInfo] detail of idempotency
// errors emitted by [defaultStatusCreator].
const (
	ReasonIDKMissing       = "IDEMPOTENCY_KEY_MISSING"
	ReasonIDKInvalidFormat = "IDEMPOTENCY_KEY_INVALID_FORMAT"
	ReasonIDKInProgress    = "IDEMPOTENCY_KEY_IN_PROGRESS"
	ReasonIDKAlreadyUsed   = "IDEMPOTENCY_KEY_ALREADY_USED"
	ReasonIDKMismatch      = "IDEMPOTENCY_KEY_MISMATCH"
	ReasonIDKForbidden     = "IDEMPOTENCY_KEY_FORBIDDEN"
)

// KeyFormatValidator validates the format of an idempotency key.
// Returns nil if the key is valid, or ErrInvalidFormat if invalid.
type KeyFormatValidator func(key string) error

// ErrInvalidFormat is returned when the idempotency key format is invalid.
var ErrInvalidFormat = internalidem.ErrInvalidFormat

// DefaultKeyValidator validates that the key is a valid lowercase UUID v4.
func DefaultKeyValidator(key string) error {
	return internalidem.DefaultKeyValidator(key)
}

// DefaultMaxReplaySize bounds a response stored for [WithResponseReplay], in
// bytes of the marshaled message (256 KiB).
const DefaultMaxReplaySize = internalidem.DefaultMaxReplaySize

// DefaultMaxKeyLength bounds the keys [OpaqueKeyValidator] and
// [PatternKeyValidator] accept, in bytes.
const DefaultMaxKeyLength = internalidem.DefaultMaxKeyLength

// OpaqueKeyValidator returns a [KeyFormatValidator] accepting any key of 1
// to maxLen bytes of visible ASCII (no space, control or non-ASCII byte), for
// clients that send their own request IDs rather than UUIDs. A maxLen below 1
// is [DefaultMaxKeyLength].
func OpaqueKeyValidator(maxLen int) KeyFormatValidator {
	return internalidem.OpaqueKeyValidator(maxLen)
}

// PatternKeyValidator returns a [KeyFormatValidator] accepting a key of 1 to
// maxLen bytes that pattern matches in full. A maxLen below 1 is
// [DefaultMaxKeyLength].
func PatternKeyValidator(pattern *regexp.Regexp, maxLen int) KeyFormatValidator {
	return internalidem.PatternKeyValidator(pattern, maxLen)
}

// Policy tells the interceptor how one method treats idempotency keys; see
// [WithMethodClassifier].
type Policy = internalidem.Policy

const (
	// PolicyDefault applies [WithEnforceMandatory]: the key is checked when
	// present and required when enforcement is on.
	PolicyDefault = internalidem.PolicyDefault
	// PolicySkip ignores idempotency for the method, as [WithIgnoreMethods]
	// does.
	PolicySkip = internalidem.PolicySkip
	// PolicyOptional checks a key when present and never requires one.
	PolicyOptional = internalidem.PolicyOptional
	// PolicyMandatory requires a key.
	PolicyMandatory = internalidem.PolicyMandatory
)

// MethodClassifier returns the [Policy] of a fully qualified method name
// ("/package.Service/Method"). It runs after the ignore lists.
type MethodClassifier func(fullMethod string) Policy

// DefaultSkipVerbs are the method-name prefixes [VerbMethodClassifier] skips
// unless [WithSkipVerbs] replaces them: Get, List and Search read, Update and
// Delete are guarded by etags or idempotent by definition, and so are their
// Batch forms (BatchGet, BatchUpdate, BatchDelete).
var DefaultSkipVerbs = internalidem.DefaultSkipVerbs

// VerbMethodClassifier returns a [MethodClassifier] that classifies a method
// by the verb its name starts with: a method whose name begins with a skip
// verb as a whole word ("GetOrder", not "Getaway") is skipped, and every
// other method — Create, BatchCreate and custom verbs such as Cancel or
// Approve, which change state on each call — gets p. The skip verbs are
// [DefaultSkipVerbs] unless [WithSkipVerbs] supplies others.
// VerbMethodClassifier(PolicyMandatory) requires a key on exactly the
// methods that are not naturally idempotent.
func VerbMethodClassifier(p Policy, opts ...VerbOption) MethodClassifier {
	verbs := slices.Clone(newVerbOptions(opts...).skipVerbs)
	return func(fullMethod string) Policy { return internalidem.VerbMethodPolicy(fullMethod, p, verbs) }
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
	exposeEntityID                 bool               `optgen:"default=false"`
	keyFormatValidator             KeyFormatValidator `optgen:"default=DefaultKeyValidator"`
	entityIdExtractor              EntityIdExtractor
	statusCreator                  StatusCreator
	// requestFingerprint records a fingerprint of the method and the
	// deterministically marshaled request with each key, so a key reused
	// for a different request is refused with ErrorIDKMismatch.
	requestFingerprint bool `optgen:"default=true" optval:"param"`
	// keyField names a string field of the request message that carries the
	// key (such as request_id), by proto or JSON name. A non-empty value in
	// the field takes precedence over the header; "" (the default) reads the
	// header only.
	keyField string
	// methodClassifier decides per method whether the key is skipped,
	// optional or mandatory; nil applies enforceMandatory to every method
	// that is not ignored.
	methodClassifier MethodClassifier
	// responseReplay stores the response of a completed unary call and
	// returns it to a repeat of the request instead of the
	// already-used error. Off by default: the repeat's caller receives the
	// original response, so scope keys per principal (idempotency.WithOwner).
	responseReplay bool
	// maxReplaySize bounds a stored response (the marshaled message); a
	// larger one is not stored and its repeat is refused as already used.
	maxReplaySize int `optgen:"default=DefaultMaxReplaySize"`
}

// EntityIdExtractor serves for extracting entity ID from the response.
type EntityIdExtractor func(method string, resp any) string

// StatusCreator serves for creating a custom gRPC status error for idempotency scenarios.
type StatusCreator func(ctx context.Context, scenario ErrorScenario, state *idempotency.State) *status.Status
