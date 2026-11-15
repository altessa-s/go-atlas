// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the mongo block of the scheduler storage
// configuration to a MongoDB task storage for factory/ext/horai,
// creating the indexes at startup when ensureIndexes is set.
//
// The scheduler factory builds only the memory and sqldb task storages;
// register this one for the mongo type:
//
//	sched, err := schedulerfactory.New(&cfg.Scheduler).
//		UseStorage(schedulerconfig.StorageTypeMongo, mongofactory.Builder(db)).
//		Build(ctx)
package factory
