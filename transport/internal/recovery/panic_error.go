// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"fmt"
)

// PanicError wraps a recovered panic value together with the stack trace
// captured at the point of recovery. It implements the error interface so
// it can be returned from interceptors and middlewares.
type PanicError struct {
	// Panic is the value that was passed to panic().
	Panic any
	// Frames contains the stack trace captured at the point of panic recovery.
	Frames Frames
}

// Error implements the error interface.
func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Panic)
}

// NewPanicError creates a [PanicError] from the recovered value p and
// captures the current goroutine's stack. skip controls how many caller
// frames above NewPanicError to omit; pass 0 to start at the caller's
// frame (the deferred recovery function).
func NewPanicError(p any, skip int) *PanicError {
	return &PanicError{
		Panic:  p,
		Frames: StackTrace(skip + 1),
	}
}
