// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build go1.26

package errors

import "errors"

// AsType delegates to [errors.AsType] from the standard library (Go 1.26+).
//
// It is a generic alternative to [errors.As] that returns the matched error
// value directly, eliminating the need for a separate target variable declaration.
//
// Instead of:
//
//	var target *AppError
//	if errors.As(err, &target) {
//	    fmt.Println("application error:", target)
//	}
//
// You can write:
//
//	if target, ok := errors.AsType[*AppError](err); ok {
//	    fmt.Println("application error:", target)
//	}
func AsType[E error](err error) (E, bool) {
	return errors.AsType[E](err)
}
