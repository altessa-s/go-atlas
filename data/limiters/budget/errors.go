// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget

import "errors"

var (
	// ErrBudgetExhausted is returned by [Limiter.Allow] when the request budget
	// for the current period has been fully consumed.
	ErrBudgetExhausted = errors.New("budget exhausted for current period")

	// ErrInvalidLimit is returned by [New] when the limit is not positive.
	ErrInvalidLimit = errors.New("budget limit must be positive")

	// ErrInvalidPeriod is returned by [New] when the period is shorter than
	// [MinPeriod].
	ErrInvalidPeriod = errors.New("budget period is too short")

	// ErrNilStorage is returned by [New] when the storage is nil.
	ErrNilStorage = errors.New("budget storage is required")
)
