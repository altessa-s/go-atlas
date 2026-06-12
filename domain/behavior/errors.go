// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import (
	"errors"
	"fmt"
	"strings"
)

// ErrMaxDepthExceeded is returned when [Strip] traverses more nested levels
// than [WithMaxDepth] permits. The cap bounds stack growth on self-referential
// or maliciously deep structs.
var ErrMaxDepthExceeded = errors.New("behavior: max traversal depth exceeded")

// Violation describes a single populated field that would have been stripped
// under [WithStrict].
type Violation struct {
	// Path is the dot-separated path to the offending field. Slice and map
	// entries are indexed ("aliases[2].id", `labels["foo"].id`).
	Path string

	// Kind is the field-behavior that triggered the violation. When a
	// field carries several kinds, it is the first one that intersects the
	// configured strip set.
	Kind Kind
}

// ViolationError aggregates every field whose value would have been cleared by
// [Strip] under [WithStrict]. The struct is left untouched when this error is
// returned.
type ViolationError struct {
	Violations []Violation
}

func (e *ViolationError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "behavior: %d violation(s)", len(e.Violations))

	for i, v := range e.Violations {
		sep := "; "
		if i == 0 {
			sep = ": "
		}

		fmt.Fprintf(&sb, "%s%s (%s)", sep, v.Path, v.Kind.String())
	}

	return sb.String()
}
