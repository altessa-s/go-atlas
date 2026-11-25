// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import "time"

// The zero clock uses Go's monotonic clock for elapsed time.
type freshnessClock struct {
	now   func() time.Time
	since func(time.Time) time.Duration
}

func (c freshnessClock) observe() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c freshnessClock) elapsed(observed time.Time) time.Duration {
	if c.since != nil {
		return c.since(observed)
	}
	return time.Since(observed)
}

type freshnessRecord struct {
	observedAt time.Time
	baseAge    time.Duration
}

func (r *freshnessRecord) fresh(clock freshnessClock, budget time.Duration) bool {
	return r != nil && clock.elapsed(r.observedAt) <= budget-r.baseAge
}
