// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package errstatus provides gRPC interceptors for consistent error-to-status
// and status-to-error conversion.
//
// Server-side: Use [ServerInterceptor] (or the standalone helpers) with
// [ErrorConverter] mappings and an optional [Finalizer] to translate Go
// errors into gRPC status codes. [DefaultFinalizer] is used when none is
// provided. The server interceptor caches error-to-status lookups with an
// internal LRU for high-throughput paths.
//
// [WithErrorRules] maps sentinels to codes with fixed messages and stable
// ErrorInfo reasons; [WithFieldErrorType] turns a domain field error into an
// InvalidArgument status with one io.altessa.badrequest.v1.BadRequest field
// violation ([BadRequest]), so handlers return domain errors unconverted; a
// [PathMapper] such as [CamelPath] or [Prefixed] spells the violation's path.
// [WithCanonicalRules] maps the canonical core/errors sentinels (ErrNotFound,
// ErrConflict, FieldError, ...) to codes with fixed messages and stable
// reasons; service-specific rules are registered after it. A request-field
// failure (page token, page size, update mask, etag) is returned as a
// core/errors.FieldError wrapping the underlying error and reaches the
// client as the same violation [FieldViolation] builds.
//
// Client-side: Use [ClientInterceptor] with a [StatusConverter] to translate
// gRPC status errors back into application-level errors.
//
// [GrpcStatusToReasonCode] maps standard gRPC codes to human-readable reason
// strings for use in error details.
//
// Example:
//
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(errstatus.ServerUnaryInterceptor(
//	        errstatus.WithErrorMapping(ErrNotFound, codes.NotFound, "Not found"),
//	    )),
//	)
package errstatus
