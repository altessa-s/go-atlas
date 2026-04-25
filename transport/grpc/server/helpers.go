// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import "google.golang.org/grpc/codes"

// CodeToString converts a gRPC status code to a capitalized, space-separated
// human-readable label (e.g. codes.NotFound becomes "Not Found").
// Unrecognized codes map to "Unknown".
func CodeToString(c codes.Code) string {
	switch c {
	case codes.OK:
		return "Ok"
	case codes.Canceled:
		return "Canceled"
	case codes.Unknown:
		return "Unknown"
	case codes.InvalidArgument:
		return "Invalid Argument"
	case codes.DeadlineExceeded:
		return "Deadline Exceeded"
	case codes.NotFound:
		return "Not Found"
	case codes.AlreadyExists:
		return "Already Exists"
	case codes.PermissionDenied:
		return "Permission Denied"
	case codes.ResourceExhausted:
		return "Resource Exhausted"
	case codes.FailedPrecondition:
		return "Failed Precondition"
	case codes.Aborted:
		return "Aborted"
	case codes.OutOfRange:
		return "Out Of Range"
	case codes.Unimplemented:
		return "Unimplemented"
	case codes.Internal:
		return "Internal"
	case codes.Unavailable:
		return "Unavailable"
	case codes.DataLoss:
		return "Data Loss"
	case codes.Unauthenticated:
		return "Unauthenticated"
	default:
		return "Unknown"
	}
}

// AllCodes enumerates the 17 standard gRPC status codes (OK through
// Unauthenticated) in numeric order.
var AllCodes = []codes.Code{
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
