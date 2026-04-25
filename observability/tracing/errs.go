// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"errors"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Sentinel errors for tracing operations.
var (
	// ErrInvalidTraceID is returned when a trace ID does not conform to the
	// 32-character hex format required by W3C Trace Context.
	ErrInvalidTraceID = errors.New("invalid trace ID")

	// ErrInvalidSpanID is returned when a span ID does not conform to the
	// 16-character hex format required by W3C Trace Context.
	ErrInvalidSpanID = errors.New("invalid span ID")

	// ErrTracerShutdown is returned by [Tracer.ForceFlush] and other operations
	// after [Tracer.Shutdown] has been called.
	ErrTracerShutdown = errors.New("tracer has been shut down")

	// ErrEmptySpanName is returned when a span name is empty.
	ErrEmptySpanName = errors.New("span name cannot be empty")

	// ErrNilAdapter is returned when a nil [adapters.Adapter] is provided.
	ErrNilAdapter = errors.New("adapter cannot be nil")
)

// WrapAdapterError wraps an adapter error with context.
// Uses core/errors for consistent error wrapping.
func WrapAdapterError(err error, adapterName string) error {
	return coreerrs.WrapOperationWithContext(err, "export spans", adapterName)
}

// WrapSpanError wraps a span error with operation context.
// Uses core/errors for consistent error wrapping.
func WrapSpanError(err error, operation string) error {
	return coreerrs.WrapOperation(err, operation)
}

// WrapFlushError wraps a flush error with adapter context.
func WrapFlushError(err error, adapterName string) error {
	return coreerrs.WrapOperationWithContext(err, "flush spans", adapterName)
}

// WrapShutdownError wraps a shutdown error with adapter context.
func WrapShutdownError(err error, adapterName string) error {
	return coreerrs.WrapOperationWithContext(err, "shutdown adapter", adapterName)
}
