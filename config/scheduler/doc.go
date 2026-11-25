// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package schedulerconfig defines the distributed scheduler schema.
//
// Schemas carry yaml and default tags read by
// github.com/altessa-s/go-nomos/loader, expose Default* constructors
// where defaults exist, and implement Validate; component factories map them
// to generated options.
//
// Key types: [ConcurrencyStrategy], [MemoryAwareConcurrency],
// [AdaptiveConcurrency], [Concurrency], [TaskConcurrency],
// [StorageType]. The go-horai factory builds the runtime limit function from [Concurrency].
//
// [ScheduleRule] validates a cron schedule spec with the grammar every
// go-horai scheduler parses (github.com/altessa-s/go-horai/schedule), so
// other schemas can check their job schedules at load time:
//
//	validation.Field(&c.Schedule, validation.Required, schedulerconfig.ScheduleRule)
package schedulerconfig
