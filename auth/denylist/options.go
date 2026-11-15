// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package denylist

//go:generate go tool optgen generate

import "time"

// options configures a [Denylist].
type options struct {
	// clock is the time source used for expiry comparisons and sweeping. It is
	// intended for tests; production uses [time.Now]. A nil clock is ignored.
	clock func() time.Time `optgen:"default=time.Now,notnil"`
}
