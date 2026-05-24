// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

import "errors"

// Sentinel errors returned by [TokenStore] implementations.
var (
	// ErrInvalidToken is returned when the token is not registered with the store.
	ErrInvalidToken = errors.New("auth/static: invalid token")

	// ErrEmptyToken is returned when an empty token is supplied.
	ErrEmptyToken = errors.New("auth/static: empty token")

	// ErrRateLimited is returned when [RateLimitedStore] rejects a request
	// because the supplied [RateLimiter] denies it.
	ErrRateLimited = errors.New("auth/static: too many authentication attempts")
)
