// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package bloom provides a Bloom filter implementation for probabilistic existence checks.
// Bloom filters are space-efficient but do not support deletion. Use periodic rebuilds
// when the underlying dataset changes; [Filter.Rebuild] replaces the contents
// atomically and keeps values added while it runs.
//
// Example:
//
//	storage := memory.New(memory.WithExpectedItems(100000))
//	filter := bloom.New(storage)
//	filter.Add(ctx, "user:123")
//	exists, _ := filter.MightExist(ctx, "user:123") // true
//
// # Rebuild lifecycle
//
// The options of [New] configure an optional rebuild lifecycle that
// [Filter.StartRebuild] starts: with a data loader ([WithRebuildLoader]) it
// rebuilds the filter synchronously ([WithRebuildOnStart]) and runs the
// periodic rebuild ([WithRebuildSchedule]) where the contents live — a
// process-local cron for a process-local storage, stopped when the filter is
// closed, or a task registered with the scheduler ([WithScheduler]) for a
// shared storage such as Redis. A filter that never calls StartRebuild keeps
// [Filter.Rebuild] and [Filter.Done] for an externally owned schedule.
//
//	filter := bloom.New(storage,
//		bloom.WithName("users"),
//		bloom.WithRebuildLoader(loader),
//		bloom.WithRebuildOnStart(),
//		bloom.WithRebuildSchedule("0 0 * * * *"),
//		bloom.WithScheduler(scheduler),
//	)
//	if err := filter.StartRebuild(ctx); err != nil {
//		_ = filter.Close(context.WithoutCancel(ctx))
//		return err
//	}
package bloom
