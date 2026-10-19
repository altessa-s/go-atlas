// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package logger provides gRPC interceptors for request and response logging.
// It captures method calls, timing, errors, and custom fields with structured logging support.
//
// # Payload redaction
//
// Request and response payloads are logged only with [WithLogRequest] / [WithLogResponse].
// [WithDebugRedact] clears the fields marked with the protobuf debug_redact option from the
// logged copy ([DebugRedact]); [WithPayloadRedactor] applies a custom [PayloadRedactor] after it.
//
// Example:
//
//	myLogger := logger.LoggerFunc(func(ctx context.Context, msg string, code codes.Code, fields logfields.Fields) {
//	    log.Printf("%s: code=%v fields=%v", msg, code, fields)
//	})
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(logger.ServerInterceptor(myLogger).ServerUnaryInterceptor()),
//	)
package logger
