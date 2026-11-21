// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package watermark

import (
	"sync"
	"time"
)

// Clock records the greatest observed caller time. Its zero value is ready for use.
type Clock struct {
	mu       sync.Mutex
	latest   time.Time
	observed bool
}

// Observe advances the clock when now is later than the last observation.
func (c *Clock) Observe(now time.Time) {
	// Entries persist wall time. Discard the process-local monotonic reading
	// before comparing observations from the caller.
	now = now.Round(0)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.observed || now.After(c.latest) {
		c.latest = now
		c.observed = true
	}
}

// Now returns the latest observation, or the zero time before the first call.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest
}
