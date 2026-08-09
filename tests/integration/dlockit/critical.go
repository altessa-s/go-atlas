// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlockit

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Visit is one holder's stay inside the critical section.
type Visit struct {
	Holder string
	Enter  time.Time
	Leave  time.Time
}

// Overlaps reports whether two visits were ever inside the section at the same
// time. Touching endpoints do not overlap: a handover where one holder leaves
// at the exact instant the next enters is correct.
func (v Visit) Overlaps(other Visit) bool {
	return v.Enter.Before(other.Leave) && other.Enter.Before(v.Leave)
}

// Critical records entries into and exits from a critical section, and reports
// whether any two of them ever coincided.
//
// It records rather than samples: every holder reports both edges, so an
// overlap of any duration is caught rather than merely likely to be caught.
type Critical struct {
	mu     sync.Mutex
	visits []Visit

	// concurrent tracks how many holders are inside right now, so a breach is
	// also observable at the moment it happens and not only in hindsight.
	concurrent atomic.Int64
	peak       atomic.Int64
}

// Enter records that holder has entered the section and returns the function
// that records its exit. The returned function must be called exactly once.
func (c *Critical) Enter(holder string) func() {
	enter := time.Now()

	if n := c.concurrent.Add(1); n > c.peak.Load() {
		c.peak.Store(n)
	}

	return func() {
		leave := time.Now()
		c.concurrent.Add(-1)

		c.mu.Lock()
		c.visits = append(c.visits, Visit{Holder: holder, Enter: enter, Leave: leave})
		c.mu.Unlock()
	}
}

// Peak returns the highest number of holders observed inside the section at
// once. Anything above one is a mutual-exclusion failure.
func (c *Critical) Peak() int {
	return int(c.peak.Load())
}

// Visits returns the recording.
func (c *Critical) Visits() []Visit {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]Visit(nil), c.visits...)
}

// Breaches returns every pair of visits that coincided.
func (c *Critical) Breaches() [][2]Visit {
	visits := c.Visits()

	var out [][2]Visit
	for i := range visits {
		for j := i + 1; j < len(visits); j++ {
			if visits[i].Overlaps(visits[j]) {
				out = append(out, [2]Visit{visits[i], visits[j]})
			}
		}
	}

	return out
}

// Timeline renders the recording for a failure message.
func (c *Critical) Timeline() string {
	var b strings.Builder

	visits := c.Visits()
	fmt.Fprintf(&b, "visits: %d, peak concurrency: %d\n", len(visits), c.Peak())

	for _, v := range visits {
		fmt.Fprintf(&b, "  %s: %s .. %s (%v)\n",
			v.Holder,
			v.Enter.Format("15:04:05.000"),
			v.Leave.Format("15:04:05.000"),
			v.Leave.Sub(v.Enter).Round(time.Millisecond))
	}

	for _, pair := range c.Breaches() {
		fmt.Fprintf(&b, "  BREACH: %s and %s overlapped\n", pair[0].Holder, pair[1].Holder)
	}

	return b.String()
}
