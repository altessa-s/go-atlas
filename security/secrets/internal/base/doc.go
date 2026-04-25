// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package base provides common utilities and patterns for secret storage providers.
//
// This package contains reusable components that implement common patterns across
// all secret storage providers:
//
//   - SingleflightGroup: embeddable wrapper around singleflight.Group with
//     convenience methods for key generation
//
//   - LockerWrapper: handles distributed locking for write operations
//
// These utilities reduce code duplication and ensure consistent behavior across
// vault, gcp, lockbox, and other secret storage implementations.
package base
