// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"github.com/altessa-s/go-atlas-core/text/strings/intern"
)

// FieldKey names a structured log attribute.
type FieldKey = string

// gRPC-specific field keys.
// Interned for memory efficiency - these strings are used frequently
// across many log entries and should be deduplicated in memory.
var (
	FieldKeyGrpcMethod      FieldKey = intern.String("grpc.method")
	FieldKeyGrpcStream      FieldKey = intern.String("grpc.stream")
	FieldKeyGrpcService     FieldKey = intern.String("grpc.service")
	FieldKeyGrpcCode        FieldKey = intern.String("grpc.code")
	FieldKeyGrpcMessage     FieldKey = intern.String("grpc.message")
	FieldKeyGrpcErrorReason FieldKey = intern.String("grpc.error_reason")
	FieldKeyResponseHeaders FieldKey = intern.String("grpc.response.headers")
)
