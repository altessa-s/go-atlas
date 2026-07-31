// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency provides middleware that prevents duplicate processing
// of non-safe HTTP requests (POST, PUT, PATCH, DELETE).
//
// Clients send an Idempotency-Key header (lowercase UUID v4 by default).
// The middleware acquires a lock in the configured [idempotency.Idempotency]
// storage before the handler runs. On success the key is marked complete;
// on failure it is deleted so the client can retry.
//
// Safe methods (GET, HEAD, OPTIONS) are idempotent by definition and bypass
// the check entirely.
//
// The middleware supports configurable fallback behavior when storage is
// unavailable: allow the request through, deny with 503, or fail with 500.
//
// # Security
//
// Keys are caller-chosen and may embed user data, and the invalid-format
// branch sees values that failed validation. [WithKeyLogMode] controls how the
// key reaches debug-level logs: [KeyLogHashed] (the default) logs a truncated
// SHA-256 digest as "key_hash", [KeyLogFull] logs the raw key as "key", and
// [KeyLogOff] omits it. Hashing is log hygiene rather than a privacy
// guarantee — a key from a small or guessable set can be recovered by hashing
// candidates. For end-to-end redaction, run production loggers at Info or
// above, or wrap the handler with the masking handler from
// [github.com/altessa-s/go-atlas/observability/slog/handler/masking].
//
// # Example
//
//	mw := idempotency.New(storage,
//	    idempotency.WithEnforceMandatory(true),
//	)
//	handler := mw.Handler(yourHandler)
package idempotency
