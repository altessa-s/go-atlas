// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package generation

import (
	"context"
	"time"
)

// Store maps a namespace to a generation number that only grows. Caches embed
// the generation in their keys; a bump orphans every entry stored under an
// older generation at once, on every replica that reads the same store.
//
// Implementations must be safe for concurrent use and must keep these
// guarantees:
//
//   - Monotonic: for one namespace, a value returned by Current or Bump is
//     never smaller than a value the store returned before it. After the
//     backing data is lost this rests on two assumptions: a fresh namespace
//     is seeded from a clock in Unix microseconds ([Seed]) instead of zero,
//     and that clock is ahead of every generation used before the loss (it
//     does not run backwards, and bumps did not outpace it). A backend that
//     restores an older value instead (a failover to a lagging replica) can
//     only fail closed where it knows a newer value; a shared backend
//     documents its residual regression window (see the Regression and
//     Residual window notes in the README of storages/redis).
//   - Bounded staleness: once Bump returns, every Current call that starts
//     later than the backend's documented staleness bound returns at least
//     the bumped value (zero for an in-process store).
//   - Fail closed: Current returns an error rather than a value older than
//     that bound. Callers treat an error as "generation unknown" and skip the
//     cache, reading the source of truth instead.
type Store interface {
	// Current returns the current generation of namespace. A namespace that
	// was never bumped has a generation too (its seed).
	Current(ctx context.Context, namespace string) (uint64, error)

	// Bump advances the generation of namespace and returns the new value.
	// An error means other readers may not have observed the bump.
	Bump(ctx context.Context, namespace string) (uint64, error)
}

// Seed returns the initial generation of a namespace first seen at now: the
// Unix time in microseconds. Seeding from the clock rather than zero keeps
// generations growing across a lost or restarted backend, so an entry cached
// under a generation from before the loss is never addressed again, as long
// as the clocks of the processes seeding a namespace do not run backwards by
// more than the time between two seedings.
func Seed(now time.Time) uint64 {
	return uint64(max(now.UnixMicro(), 0))
}
