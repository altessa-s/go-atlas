// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-memory implementation of [scheduler.Storage] for
// the scheduler service. All stored data lives in process memory and is lost when
// the process exits, making this backend suitable for tests, local development,
// and single-instance deployments where persistence is not required.
//
// [Storage] is safe for concurrent use; every public method acquires the internal
// mutex before accessing the underlying maps. Returned values are deep-copied so
// callers may mutate them freely without affecting stored state.
//
// History entries are bounded per task by the limit passed to [New]. When the
// limit is exceeded, the oldest entry is evicted on the next [Storage.AddHistory]
// call.
//
// Example:
//
//	storage := memory.New(100)
//	sched := scheduler.New(storage)
package memory
