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
// # Example
//
//	mw := idempotency.New(storage,
//	    idempotency.WithEnforceMandatory(),
//	)
//	handler := mw.Handler(yourHandler)
package idempotency
