// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/internal/transport/validation"

	corecontext "github.com/altessa-s/go-atlas-core/context"
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

// FinalizeTimeout bounds the Complete or Release call that ends a key's
// lifecycle once its request was served.
const FinalizeTimeout = 5 * time.Second

// FinalizeContext returns the context a transport completes or releases a key
// under: ctx without its cancellation and deadline, so a client that
// disconnected cannot leave the key in progress, bounded by [FinalizeTimeout].
func FinalizeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return corecontext.ApplyTimeout(context.WithoutCancel(ctx), FinalizeTimeout)
}

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
