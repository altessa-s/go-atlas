// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

import "errors"

// Sentinel errors returned by [TokenStore] implementations.
var (
	// ErrTokenInvalid is returned when the token is not registered with the store.
	ErrTokenInvalid = errors.New("auth/static: invalid token")

	// ErrTokenEmpty is returned when an empty token is supplied.
	ErrTokenEmpty = errors.New("auth/static: empty token")

	// ErrRateLimited is returned when [RateLimitedStore] rejects a request
	// because the supplied [RateLimiter] denies it.
	ErrRateLimited = errors.New("auth/static: too many authentication attempts")

	// ErrRateLimitKeyEmpty is returned when the [KeyFunc] of a
	// [RateLimitedStore] yields an empty key. The request is rejected without
	// validating the token (fail closed): a missing key means the caller cannot
	// be attributed to a rate-limit bucket.
	ErrRateLimitKeyEmpty = errors.New("auth/static: empty rate-limit key")
)
