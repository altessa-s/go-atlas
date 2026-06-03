// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior

import (
	"errors"
	"fmt"
	"strings"

	"github.com/altessa-s/go-atlas/domain/proto/internal/behavior"
)

// ErrMaxDepthExceeded is returned when [Strip] traverses more nested levels
// than [WithMaxDepth] permits. Protocol Buffers schemas may not contain
// cycles, so hitting the limit indicates a malformed or fuzzed descriptor.
var ErrMaxDepthExceeded = errors.New("fieldbehavior: max traversal depth exceeded")

// BehaviorViolation describes a single populated field that would have been
// stripped in non-strict mode. It aliases [behavior.Violation] so the gRPC
// interceptor can render fieldbehavior and fieldmask violations through a
// single google.rpc.BadRequest FieldViolation mapping. The Reason field of
// [behavior.Violation] is left empty by fieldbehavior — strict-mode strip
// conveys intent through Behavior alone.
type BehaviorViolation = behavior.Violation

// BehaviorViolationError aggregates every field whose value would have been
// cleared by [Strip] under [WithStrict]. The message is left untouched when
// this error is returned.
type BehaviorViolationError struct {
	Violations []BehaviorViolation
}

func (e *BehaviorViolationError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "fieldbehavior: %d violation(s)", len(e.Violations))

	for i, v := range e.Violations {
		sep := "; "
		if i == 0 {
			sep = ": "
		}

		fmt.Fprintf(&sb, "%s%s (%s)", sep, v.Path, v.Behavior.String())
	}

	return sb.String()
}
