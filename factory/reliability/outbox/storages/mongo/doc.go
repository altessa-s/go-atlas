// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps a MongoDB database or collection to a MongoDB outbox
// storage for factory/reliability/outbox, creating the indexes on the build context.
//
// The outbox factory builds only the SQL storage; pass this one to
// BuildWithStorage:
//
//	ob, err := outboxfactory.New(&cfg.Outbox).
//		UseScheduler(scheduler).
//		BuildWithStorage(ctx, outboxmongofactory.Builder(db), handler)
package factory
