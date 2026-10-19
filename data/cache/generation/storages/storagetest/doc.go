// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the backend-neutral contract suite for
// [generation.Store] implementations. Every bundled backend runs it, and a
// custom backend should too: generational caches rest on seeding from a clock,
// bumps visible at once to the bumper and within the documented bound to
// every replica, and generations that never go backwards.
//
// [Run] takes a [NewBackend] factory and the backend's staleness bound, and
// executes every contract as parallel subtests on a fresh backend each. Each
// contract is also exported on its own, with the same arguments, and fails the
// test on any violation. The suite advances the staleness clock strictly past
// the bound, since a store guarantees visibility only to calls starting later
// than it.
//
// # Clocks
//
// A [Backend] exposes two clocks. SetSeedTime controls the clock fresh
// namespaces are seeded from: the injected clock of storages/memory, the Redis
// server clock (miniredis) of storages/redis. Advance moves the clock that
// measures local staleness, the injected clock of storages/redis; a backend
// with no staleness bound leaves it nil. A backend whose seed clock cannot be
// controlled (a live server) leaves SetSeedTime nil and the seed contract
// skips.
//
// # Usage
//
//	func TestStore_Contract(t *testing.T) {
//		t.Parallel()
//		storagetest.Run(t, newBackend, time.Second) // newBackend(tb, replicas) storagetest.Backend
//	}
package storagetest
