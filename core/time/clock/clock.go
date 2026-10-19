// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clock

import (
	"context"
	"sync"
	"time"
)

// Clock is a source of the current time that can also wait. Inject it where code reads the time or sleeps, so tests can drive both
// with a [Fake]. Its method values fit the usual seams: Now as a func() time.Time (keyset.WithClock and the like), Sleep as a
// retry.SleepFunc.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// Sleep waits d, returning ctx.Err() as soon as ctx ends (also when d is not positive and ctx has already ended).
	Sleep(ctx context.Context, d time.Duration) error
}

// Real returns the system clock: time.Now and a timer-backed Sleep.
func Real() Clock { return realClock{} }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Fake is a manually driven [Clock] for tests: Now returns the set time, Advance and Set move it, and Sleep advances it by the
// requested duration at once instead of waiting. It is safe for concurrent use. The zero Fake reads the zero time.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

var _ Clock = (*Fake)(nil)

// NewFake returns a fake clock reading now.
func NewFake(now time.Time) *Fake { return &Fake{now: now} }

// Now returns the fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance moves the fake time forward by d (backward for a negative d).
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Set sets the fake time.
func (f *Fake) Set(now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = now
}

// Sleep advances the fake time by d (when positive) without waiting and returns ctx.Err(): a sleep in an ended context still moves
// the time, as a real sleep may have run for part of d.
func (f *Fake) Sleep(ctx context.Context, d time.Duration) error {
	if d > 0 {
		f.Advance(d)
	}
	return ctx.Err()
}
