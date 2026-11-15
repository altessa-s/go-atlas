// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cfgerrors

import (
	"fmt"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// Invalidf formats like [fmt.Errorf] and classifies the result as
// [coreerrs.ErrInvalidArgument]: the message is exactly the formatted text,
// and [errors.Is] matches the sentinel as well as any error wrapped with %w.
func Invalidf(format string, args ...any) error {
	return &invalidError{err: fmt.Errorf(format, args...)}
}

// invalidError carries the message of err and adds
// [coreerrs.ErrInvalidArgument] to its chain.
type invalidError struct {
	err error
}

func (e *invalidError) Error() string { return e.err.Error() }

func (e *invalidError) Unwrap() []error { return []error{coreerrs.ErrInvalidArgument, e.err} }
