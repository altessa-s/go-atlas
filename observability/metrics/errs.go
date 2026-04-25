// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"errors"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Sentinel errors for metrics operations.
var (
	// ErrEmptyMetricName is returned by [MetricOpts.Validate] when [MetricOpts.Name]
	// is empty.
	ErrEmptyMetricName = errors.New("metric name cannot be empty")

	// ErrEmptyLabelName is returned by [ValidateLabelNames] when a label name is empty.
	ErrEmptyLabelName = errors.New("label name cannot be empty")

	// ErrLabelCountMismatch is returned by [ValidateLabels] when the number of
	// label values doesn't match the expected label names.
	ErrLabelCountMismatch = errors.New("label count mismatch")

	// ErrMissingLabel is returned by [ValidateLabels] when a required label key
	// is absent from the provided [Labels] map.
	ErrMissingLabel = errors.New("missing required label")

	// ErrCollectorShutdown is returned by [Collector.ForceFlush] and other
	// operations after [Collector.Shutdown] has been called.
	ErrCollectorShutdown = errors.New("collector has been shut down")

	// ErrAdapterClosed is returned when an [adapters.Adapter] has been closed.
	ErrAdapterClosed = errors.New("adapter has been closed")
)

// WrapAdapterError wraps an adapter error with context.
// Uses core/errors for consistent error wrapping.
func WrapAdapterError(err error, adapterName string) error {
	return coreerrs.WrapOperationWithContext(err, "record metric", adapterName)
}

// WrapMetricError wraps a metric error with operation context.
// Uses core/errors for consistent error wrapping.
func WrapMetricError(err error, operation string) error {
	return coreerrs.WrapOperation(err, operation)
}

// WrapFlushError wraps a flush error with adapter context.
func WrapFlushError(err error, adapterName string) error {
	return coreerrs.WrapOperationWithContext(err, "flush metrics", adapterName)
}

// WrapShutdownError wraps a shutdown error with adapter context.
func WrapShutdownError(err error, adapterName string) error {
	return coreerrs.WrapOperationWithContext(err, "shutdown adapter", adapterName)
}
