// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package headers

import corestrings "github.com/altessa-s/go-atlas/core/text/strings"

// Common HTTP header names (interned for memory efficiency).
// These constants should be used across the transport layer for consistency.
var (
	// ContentType represents the Content-Type header.
	ContentType = corestrings.InternString("Content-Type")
	// ContentLength represents the Content-Length header.
	ContentLength = corestrings.InternString("Content-Length")
	// Accept represents the Accept header.
	Accept = corestrings.InternString("Accept")
)

// Rate limiting headers (interned for memory efficiency).
// These are used by the rate limiting middleware to communicate limits to clients.
var (
	// RateLimitLimit represents the X-RateLimit-Limit header.
	RateLimitLimit = corestrings.InternString("X-RateLimit-Limit")
	// RateLimitRemaining represents the X-RateLimit-Remaining header.
	RateLimitRemaining = corestrings.InternString("X-RateLimit-Remaining")
	// RateLimitReset represents the X-RateLimit-Reset header.
	RateLimitReset = corestrings.InternString("X-RateLimit-Reset")
	// RetryAfter represents the Retry-After header.
	RetryAfter = corestrings.InternString("Retry-After")
)

// Raw header name constants (not interned).
// Use these when you need the original string value.
const (
	// HeaderContentType is the Content-Type header name.
	HeaderContentType = "Content-Type"
	// HeaderContentLength is the Content-Length header name.
	HeaderContentLength = "Content-Length"
	// HeaderAccept is the Accept header name.
	HeaderAccept = "Accept"

	// HeaderRateLimitLimit is the X-RateLimit-Limit header name.
	HeaderRateLimitLimit = "X-RateLimit-Limit"
	// HeaderRateLimitRemaining is the X-RateLimit-Remaining header name.
	HeaderRateLimitRemaining = "X-RateLimit-Remaining"
	// HeaderRateLimitReset is the X-RateLimit-Reset header name.
	HeaderRateLimitReset = "X-RateLimit-Reset"
	// HeaderRetryAfter is the Retry-After header name.
	HeaderRetryAfter = "Retry-After"
)
