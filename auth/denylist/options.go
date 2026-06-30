// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package denylist

import "time"

// Option configures a [Denylist]. The denylist has no production tunables; the
// only option overrides the time source for tests, so the configuration is
// hand-written rather than generated.
type Option func(*Denylist)

// WithClock overrides the time source used for expiry comparisons and sweeping.
// It is intended for tests; production uses [time.Now]. A nil clock is ignored.
func WithClock(now func() time.Time) Option {
	return func(d *Denylist) {
		if now != nil {
			d.now = now
		}
	}
}
