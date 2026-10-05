// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package facade provides the plumbing shared by the Bloom and Cuckoo filter
// facades: atomic rebuild coordination and operation observation, combined in
// [Base], which implements the operations both facades forward to.
//
// # Rebuild
//
// A [Coordinator] builds a replacement filter through a storage-provided
// [Staging] while the live filter keeps answering lookups, journals adds made
// through the facade in the meantime, replays them onto the replacement and
// commits it in one step. A failed or canceled rebuild aborts the replacement
// and leaves the live filter untouched:
//
//	var coord facade.Coordinator
//	err := coord.Add(ctx, value, func() error { return storage.Add(ctx, value) })
//	err = coord.Rebuild(ctx, loader, func(ctx context.Context, n int64) (facade.Staging, error) {
//	    return storage.Stage(ctx, n)
//	})
//
// # Observation
//
// An [ObserverSlot] holds the facade's [probfilter.Observer] and reports
// lookups, adds and rebuilds to it; with no observer installed every report is
// a single atomic load.
package facade
