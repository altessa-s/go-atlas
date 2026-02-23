// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"context"

	"github.com/altessa-s/go-atlas/observability/tracing"
)

// FromContextOrTraceID returns the request ID from context.
// If no request ID is found, it falls back to the trace ID from the tracing context.
// Returns empty string if neither is available.
//
// This is useful for correlating logs and other observability data when
// a dedicated request ID is not available but tracing is enabled.
//
// Example:
//
//	id := requestid.FromContextOrTraceID(ctx)
//	if id != "" {
//	    logger.Info("processing request", slog.String("correlation_id", id))
//	}
func FromContextOrTraceID(ctx context.Context) string {
	// First try to get request ID
	if id := FromContext(ctx); id != "" {
		return id
	}

	// Fall back to trace ID
	return tracing.TraceIDFromContext(ctx)
}
