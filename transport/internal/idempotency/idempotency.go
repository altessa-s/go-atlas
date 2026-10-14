// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"errors"
	"strings"

	"github.com/altessa-s/go-atlas/transport/internal/validation"

	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
)

// Canonical idempotency header/metadata names shared by the HTTP middleware
// and the gRPC interceptor. The transport packages re-export them under
// their historical names.
const (
	// DefaultKeyHeader is the canonical header/metadata name carrying the
	// idempotency key. Per ADR-26-01-12-002, the name is "Idempotency-Key".
	DefaultKeyHeader = "Idempotency-Key"
	// DefaultKeyStatusHeader is the canonical header/metadata name carrying
	// the idempotency status of a duplicate request.
	DefaultKeyStatusHeader = "Idempotency-Key-Status"
	// DefaultKeyEntityIDHeader is the canonical header/metadata name carrying
	// the entity ID stored alongside a completed idempotency key.
	DefaultKeyEntityIDHeader = "Idempotency-Key-Entity-Id"
)

// ErrInvalidFormat is returned by [DefaultKeyValidator] when the idempotency
// key is not a valid lowercase UUID v4.
var ErrInvalidFormat = errors.New("invalid idempotency key format")

// DefaultKeyValidator validates that the key is a valid lowercase UUID v4.
// It returns nil for a valid key and [ErrInvalidFormat] otherwise.
func DefaultKeyValidator(key string) error {
	if strings.ToLower(key) != key || !validation.IsValidUUIDv4(key) {
		return ErrInvalidFormat
	}
	return nil
}

// BuildStorageKey constructs the storage key for an idempotency entry: the
// tuple ("transport", service, key) composed by [idempotencydata.ComposeKey], so
// distinct (service, key) pairs never share a storage key whatever bytes
// they hold. The service part is transport-specific: "{METHOD}:/{path}" for
// HTTP and "package.Service" for gRPC.
//
// The result persists in external storage backends and is an external ABI.
// The v2 layout is frozen; an incompatible format needs a new version and a
// migration.
func BuildStorageKey(service, key string) string {
	return idempotencydata.ComposeKey("transport", service, key)
}
