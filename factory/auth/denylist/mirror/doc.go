// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder that constructs a
// [github.com/altessa-s/go-atlas/auth/denylist/mirror.Cache] around a
// revocation source and wires its lifecycle.
//
// [Builder.Build] primes the cache with the build context, so a service whose
// revocation store is unreachable fails to start instead of serving tokens
// unchecked; registers the refresh task with the scheduler set by
// [Builder.UseScheduler]; and, when a health registrar is set, registers a
// checker that reports NotServing while the cache is unprimed or older than
// [Builder.UseMaxStaleness]. The cache fails closed: until it is primed, and
// whenever it is too stale, a verifier configured with it rejects tokens with
// jwt.ErrRevocationUnavailable. Each mirror registers its own task and health
// service ([Builder.UseName]) because a scheduler replaces a task registered
// under an existing ID; the maximum staleness must not be shorter than an
// "@every" refresh interval, and is off by default. [Builder.Build] returns a
// [Managed] mirror; its Close stops the refresh task, removes it from a
// scheduler that supports removal and releases the health registration.
//
//	cache, err := factory.NewBuilder(store.RawSource()).
//	    UseScheduler(scheduler).
//	    UseHealthCoordinator(coordinator).
//	    UseRefreshSchedule("@every 15s").
//	    UseMaxStaleness(time.Minute).
//	    Build(ctx)
//	if err != nil {
//	    return err
//	}
//	defer cache.Close()
//	verifier := jwt.NewVerifier(resolver, jwt.WithRevocation(cache))
package factory
