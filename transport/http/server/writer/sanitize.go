// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"errors"

	"github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"

	"google.golang.org/protobuf/proto"
)

// sanitizeResponse strips google.api.field_behavior INPUT_ONLY fields from a
// proto.Message response so input-only secrets (passwords, one-time tokens)
// accepted on the write path never leak back to clients on the read path.
//
// The caller's message is never mutated. A strict-mode detection pass reports
// whether any populated INPUT_ONLY field exists without touching msg; only when
// one is found does the sanitizer clone the message and clear the fields on the
// copy. Responses with nothing to strip — the common case — incur a single
// read-only traversal and zero allocation.
//
// Non-proto data is returned unchanged.
func sanitizeResponse(data any) (any, error) {
	msg, ok := data.(proto.Message)
	if !ok {
		return data, nil
	}

	// Detect populated INPUT_ONLY fields without mutating the original: strict
	// mode leaves msg intact and reports violations instead of clearing them.
	err := fieldbehavior.StripResponse(msg, fieldbehavior.WithStrict())
	if err == nil {
		return data, nil
	}

	var violations *fieldbehavior.BehaviorViolationError
	if !errors.As(err, &violations) {
		// A non-violation error (e.g. ErrMaxDepthExceeded) is a real failure.
		return nil, err
	}

	// At least one INPUT_ONLY field is populated: strip it on a clone so the
	// handler's original response object stays untouched.
	clone := proto.Clone(msg)
	if err := fieldbehavior.StripResponse(clone); err != nil {
		return nil, err
	}

	return clone, nil
}
