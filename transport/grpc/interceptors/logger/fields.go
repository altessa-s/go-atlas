// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"github.com/altessa-s/go-atlas-core/text/strings/intern"
	"github.com/altessa-s/go-atlas/internal/transport/observability"
)

// gRPC-specific field keys.
// Interned for memory efficiency - these strings are used frequently
// across many log entries and should be deduplicated in memory.
var (
	FieldKeyGrpcMethod      observability.FieldKey = intern.String("grpc.method")
	FieldKeyGrpcStream      observability.FieldKey = intern.String("grpc.stream")
	FieldKeyGrpcService     observability.FieldKey = intern.String("grpc.service")
	FieldKeyGrpcCode        observability.FieldKey = intern.String("grpc.code")
	FieldKeyGrpcMessage     observability.FieldKey = intern.String("grpc.message")
	FieldKeyGrpcErrorReason observability.FieldKey = intern.String("grpc.error_reason")
	FieldKeyResponseHeaders observability.FieldKey = intern.String("grpc.response.headers")
)
