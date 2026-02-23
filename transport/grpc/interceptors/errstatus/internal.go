// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errorTypeString returns a string representation of the error type for debugging.
// This is used for logging and cache key generation.
func errorTypeString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", err)
}

// errorKey generates a cache key from error type and message.
// This is used when caching all errors (not just sentinels).
func errorKey(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T:%s", err, err.Error())
}

// sentinelErrorKey generates a cache key for sentinel errors using their pointer address.
// Sentinel errors are package-level variables with stable addresses.
func sentinelErrorKey(err error) string {
	if err == nil {
		return ""
	}
	// For sentinel errors, pointer address is the most reliable identifier.
	// Sentinel errors are by definition package-level variables with stable addresses.
	return fmt.Sprintf("%T:%p", err, err)
}

// findSentinelError searches through a list of sentinel errors to find which one
// the given error matches using errors.Is(). Returns the matching sentinel or nil.
func findSentinelError(err error, sentinels []error) error {
	if err == nil {
		return nil
	}
	for _, sentinel := range sentinels {
		if sentinel == nil {
			continue
		}
		if errors.Is(err, sentinel) {
			return sentinel
		}
	}
	return nil
}

// buildStatusConverterIndex builds an optimized index of status errorConverters.
// It separates errorConverters into fast path (code-only matchers) and slow path
// (complex matchers that inspect details) for optimal performance.
func buildStatusConverterIndex(converters []StatusConverter) *statusConverterIndex {
	if len(converters) == 0 {
		return nil
	}

	idx := &statusConverterIndex{
		fastPath: make(map[codes.Code][]StatusConverter),
		slowPath: make([]StatusConverter, 0),
	}

	for _, converter := range converters {
		// Try to detect if this is a simple code-based matcher
		// We analyze the matcher to see if it can be optimized
		if code := tryExtractCodeFromMatcher(converter.Matcher); code != codes.Unknown {
			// Fast path: converter only checks status code
			idx.fastPath[code] = append(idx.fastPath[code], converter)
		} else {
			// Slow path: complex matcher (checks details, message, etc.)
			idx.slowPath = append(idx.slowPath, converter)
		}
	}

	return idx
}

// tryExtractCodeFromMatcher attempts to detect if a matcher only checks status code.
// This is a heuristic-based approach that tries a few common codes to detect patterns.
// Returns codes.Unknown if the matcher is complex and cannot be optimized.
func tryExtractCodeFromMatcher(matcher func(context.Context, *status.Status) bool) codes.Code {
	ctx := context.Background()

	// List of common gRPC codes to test
	testCodes := []codes.Code{
		codes.OK,
		codes.Canceled,
		codes.Unknown,
		codes.InvalidArgument,
		codes.DeadlineExceeded,
		codes.NotFound,
		codes.AlreadyExists,
		codes.PermissionDenied,
		codes.ResourceExhausted,
		codes.FailedPrecondition,
		codes.Aborted,
		codes.OutOfRange,
		codes.Unimplemented,
		codes.Internal,
		codes.Unavailable,
		codes.DataLoss,
		codes.Unauthenticated,
	}

	var matchedCode = codes.Unknown
	matchCount := 0

	// Test each code with empty message and no details
	for _, code := range testCodes {
		st := status.New(code, "")
		if matcher(ctx, st) {
			matchedCode = code
			matchCount++
		}
	}

	// If exactly one code matched, this is a simple code-based matcher
	if matchCount == 1 {
		return matchedCode
	}

	// If multiple or no codes matched, this is a complex matcher
	return codes.Unknown
}
