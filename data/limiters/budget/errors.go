// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget

import "errors"

// ErrBudgetExhausted is returned by [Limiter.Allow] when the request budget
// for the current period has been fully consumed.
var ErrBudgetExhausted = errors.New("budget exhausted for current period")
