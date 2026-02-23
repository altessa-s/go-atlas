// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongodb implements [scheduler.Storage] using MongoDB as the backing
// store. Task states and execution history are persisted in separate collections
// within the same database, defaulting to [DefaultTasksCollection] and
// [DefaultHistoryCollection].
//
// Collection names can be overridden with [WithTasksCollection] and
// [WithHistoryCollection]. Call [Storage.EnsureIndexes] once at startup to
// create indexes for optimal query performance.
//
// All operations are safe for concurrent use.
//
// Example:
//
//	client, _ := mongo.New("scheduler", mongo.WithMongoClientOptions(opts))
//	client.Connect(ctx)
//
//	storage := mongodb.New(client.Client().Database("scheduler"))
//	s := scheduler.New(storage)
package mongodb
