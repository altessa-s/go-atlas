// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"github.com/altessa-s/go-atlas/core/text/strings"

	"google.golang.org/grpc/codes"
)

// Pre-computed interned strings for all gRPC status codes.
// These are used to avoid repeated string allocations when logging.
var internedCodes = func() map[codes.Code]string {
	m := make(map[codes.Code]string, 18) //nolint:mnd // 17 standard codes + 1 extra
	for _, code := range []codes.Code{
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
	} {
		m[code] = strings.InternString(code.String())
	}
	return m
}()

// DefaultLogCodes contains all standard gRPC status codes that should be logged by default.
// This is pre-computed at initialization to avoid repeated slice creation.
var DefaultLogCodes = []codes.Code{
	codes.OK,
	codes.Canceled,
	codes.Unknown,
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
	codes.InvalidArgument,
}

// InternedCodeString returns the interned string representation of a gRPC code.
// If the code is not pre-computed, it falls back to intern the string dynamically.
func InternedCodeString(code codes.Code) string {
	if s, ok := internedCodes[code]; ok {
		return s
	}
	return strings.InternString(code.String())
}
