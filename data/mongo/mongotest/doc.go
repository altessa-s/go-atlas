// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongotest provides an env-gated MongoDB connection for integration tests of services built on data/mongo.
//
// [New] connects to the URI in MONGO_URI ([WithEnv] picks another variable) and returns a [datamongo.Mongo] bound to a throwaway
// database that a cleanup drops; the test is skipped when the variable is unset, so a plain go test run needs no infrastructure.
// [IndexNames] lists a collection's indexes for assertions and [RequireReplicaSet] fails a test that needs transactions on a
// standalone server.
//
//	func TestStoreIT(t *testing.T) {
//	    m := mongotest.New(t, mongotest.WithEnv("APP_IT_MONGO_URI"), mongotest.WithDatabasePrefix("app_it_"))
//	    mongotest.RequireReplicaSet(t, m)
//	    store := mystore.New(m)
//	    // ...
//	}
package mongotest
