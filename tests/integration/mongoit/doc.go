// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongoit runs data/mongo against a live MongoDB replica set: a
// transaction whose handler panics is aborted rather than committed, and a
// failing migration keeps the migrations already applied unless a full
// rollback is requested, and concurrent clients migrate their own databases.
//
// The test migrations register in the global mongo-migrate registry, as an
// application's migrations do; their file names carry the versions.
package mongoit
