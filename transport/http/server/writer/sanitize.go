// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"

	"google.golang.org/protobuf/proto"
)

// ErrResponseSanitization is returned by [Writer.Write] and [Writer.WriteStream]
// when stripping INPUT_ONLY fields from a response fails, for example because
// the message nests deeper than [WithResponseSanitizationMaxDepth]. The client
// receives a 500 response without the original payload.
var ErrResponseSanitization = errors.New("writer: response sanitization failed")

// sanitizeResponse strips google.api.field_behavior INPUT_ONLY fields from a
// proto.Message response, so input-only values (passwords, one-time tokens)
// accepted on the write path are not echoed on the read path.
//
// The caller's message is never mutated. A strict-mode pass reports whether any
// populated INPUT_ONLY field exists without touching msg; only then is the
// message cloned and the fields cleared on the copy. A response with nothing to
// strip costs one read-only traversal and no clone.
//
// Only the message passed as data is inspected. The contents of
// google.protobuf.Any fields (opaque bytes), proto messages held in slices,
// maps or caller-defined envelopes, and error bodies are not sanitized.
// Non-proto data is returned unchanged.
func sanitizeResponse(data any, maxDepth int) (any, error) {
	msg, ok := data.(proto.Message)
	if !ok {
		return data, nil
	}

	// Detect populated INPUT_ONLY fields without mutating the original: strict
	// mode leaves msg intact and reports violations instead of clearing them.
	err := fieldbehavior.StripResponse(msg, fieldbehavior.WithStrict(), fieldbehavior.WithMaxDepth(maxDepth))
	if err == nil {
		return data, nil
	}

	if _, ok := errors.AsType[*fieldbehavior.BehaviorViolationError](err); !ok {
		// A non-violation error (e.g. ErrMaxDepthExceeded) is a real failure.
		return nil, fmt.Errorf("%w: %w", ErrResponseSanitization, err)
	}

	// At least one INPUT_ONLY field is populated: strip it on a clone so the
	// handler's original response object stays untouched.
	clone := proto.Clone(msg)
	if err := fieldbehavior.StripResponse(clone, fieldbehavior.WithMaxDepth(maxDepth)); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResponseSanitization, err)
	}

	return clone, nil
}

// sanitize applies sanitizeResponse unless it is disabled. On failure it writes
// a 500 response, logs the cause and returns it.
func (wr *Writer) sanitize(w http.ResponseWriter, r *http.Request, data any) (any, bool, error) {
	if wr.options.responseSanitizationDisabled {
		return data, true, nil
	}
	sanitized, err := sanitizeResponse(data, wr.options.responseSanitizationMaxDepth)
	if err == nil {
		return sanitized, true, nil
	}
	wr.options.logger.ErrorContext(r.Context(), "writer: response sanitization failed", slog.Any("error", err))
	if writeErr := wr.writeError(w, r, ErrResponseSanitization, http.StatusInternalServerError); writeErr != nil {
		return nil, false, errors.Join(err, writeErr)
	}
	return nil, false, err
}
