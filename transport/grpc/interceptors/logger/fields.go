// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/transport/internal/observability"
)

// gRPC-specific field keys.
// Interned for memory efficiency - these strings are used frequently
// across many log entries and should be deduplicated in memory.
var (
	FieldKeyGrpcMethod      observability.FieldKey = strings.InternString("grpc.method")
	FieldKeyGrpcStream      observability.FieldKey = strings.InternString("grpc.stream")
	FieldKeyGrpcService     observability.FieldKey = strings.InternString("grpc.service")
	FieldKeyGrpcCode        observability.FieldKey = strings.InternString("grpc.code")
	FieldKeyGrpcMessage     observability.FieldKey = strings.InternString("grpc.message")
	FieldKeyGrpcErrorReason observability.FieldKey = strings.InternString("grpc.error_reason")
	FieldKeyResponseHeaders observability.FieldKey = strings.InternString("grpc.response.headers")
)
