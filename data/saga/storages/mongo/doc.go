// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides a durable [saga.Store] backed by a MongoDB collection.
//
// Each saga instance is stored as one document keyed by its ID. The document's
// version field is the optimistic-concurrency token: Update is a version-checked
// write that fails with errs.ErrVersionConflict when another coordinator has
// advanced the instance, so concurrent recovery cycles cannot double-advance it.
//
// # Indexes
//
// New creates a compound index on (status, deadline) that backs FetchRecoverable:
// the recovery scan filters by status, and by deadline for the timed-out RUNNING
// branch.
//
// # Usage
//
//	store, err := mongo.New(db, mongo.WithCollectionName("saga_instances"))
//	if err != nil {
//		return err
//	}
//	orch := saga.New(store, def, saga.WithSagaTimeout(5*time.Minute))
package mongo
