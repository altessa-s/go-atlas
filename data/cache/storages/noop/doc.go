// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package noop provides a no-operation cache provider that discards all writes.
// Useful for testing or disabling caching without code changes.
//
// Example:
//
//	p := noop.New()
//	p.Get(ctx, "key") // always returns ErrMissing
package noop
