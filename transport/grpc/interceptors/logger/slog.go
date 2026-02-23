// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// Slog returns a [Logger] that writes through the provided [slog.Logger].
// gRPC status codes are mapped to slog levels:
//
//   - Info: OK, Canceled
//   - Warn: DeadlineExceeded, NotFound, AlreadyExists, ResourceExhausted, Aborted, OutOfRange
//   - Error: all other codes (Unknown, InvalidArgument, Internal, Unavailable, etc.)
//
// Duplicate fields are deduplicated via [slogx.Fields.Unique] before emission.
func Slog(l *slog.Logger) Logger {
	return LoggerFunc(func(ctx context.Context, msg string, grpcCode codes.Code, fields slogx.Fields) {
		level := grpcCodeToLevel(grpcCode)
		attrs := slogx.FieldsToAttrs(fields.Unique())
		l.LogAttrs(ctx, level, msg, attrs...)
	})
}

func grpcCodeToLevel(code codes.Code) slog.Level {
	switch code {
	case codes.OK, codes.Canceled:
		return slog.LevelInfo
	case codes.DeadlineExceeded, codes.NotFound, codes.AlreadyExists,
		codes.ResourceExhausted, codes.Aborted, codes.OutOfRange:
		return slog.LevelWarn
	case codes.Unknown, codes.InvalidArgument, codes.Unauthenticated,
		codes.FailedPrecondition, codes.PermissionDenied, codes.Unimplemented,
		codes.Internal, codes.Unavailable, codes.DataLoss:
		return slog.LevelError
	default:
		return slog.LevelError
	}
}
