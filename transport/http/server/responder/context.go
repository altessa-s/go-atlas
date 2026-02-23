// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package responder

import (
	"context"
	"net/http"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

// ErrorWriter defines the interface for writing structured error responses.
// This minimal interface avoids import cycles with the server package.
type ErrorWriter interface {
	// WriteError writes a structured error response with content negotiation.
	WriteError(w http.ResponseWriter, r *http.Request, err error, statusCode ...int) error
}

type errorWriterContextKey struct{}

// NewContext returns a new context with the given ErrorWriter.
//
// Example:
//
//	ctx := responder.NewContext(r.Context(), writer)
//	r = r.WithContext(ctx)
func NewContext(ctx context.Context, writer ErrorWriter) context.Context {
	return context.WithValue(corecontext.OrBackground(ctx), errorWriterContextKey{}, writer)
}

// FromContext returns the ErrorWriter from the context.
// Returns nil if not found or context is nil.
//
// Example:
//
//	writer := responder.FromContext(r.Context())
//	if writer != nil {
//	    writer.WriteError(w, r, err, http.StatusBadRequest)
//	}
func FromContext(ctx context.Context) ErrorWriter {
	writer, ok := corecontext.OrBackground(ctx).Value(errorWriterContextKey{}).(ErrorWriter)
	if !ok {
		return nil
	}
	return writer
}
