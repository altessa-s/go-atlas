// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package cas provides optimistic-concurrency (compare-and-set) helpers for MongoDB documents that carry an int64 version.
//
// # Versions
//
// Every stored document carries its version. [Filter] matches a document at exactly the expected version, and [Inc] bumps the
// version by one in the same update as the data, so a write succeeds only for the writer that read the current version.
// [InitialVersion] (the insert time in nanoseconds) starts a new document above every version an earlier document with the same ID
// had, which closes the ABA window of physical deletes.
//
// # Misses
//
// A CAS write that matches nothing is either a missing document or a version conflict. [ClassifyMiss] checks which and returns the
// caller's sentinel for each, so stores keep their own error vocabulary:
//
//	update := cas.Inc("version")
//	update["$set"] = fields
//	res, err := coll.UpdateOne(ctx, cas.Filter("_id", id, "version", expected), update)
//	if err != nil {
//	    return err
//	}
//	if res.MatchedCount == 0 {
//	    return cas.ClassifyMiss(ctx, coll, "_id", id, ErrNotFound, ErrVersionConflict)
//	}
package cas
