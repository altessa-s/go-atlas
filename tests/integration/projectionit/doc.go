// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package projectionit runs data/projection against live backends.
//
// The unit tests pin the projection documents and column lists byte for
// byte; this suite checks that the servers accept them and return what the
// policy promises:
//
//   - MongoDB: an inclusion projection with a parent and a child path does
//     not trip "Path collision"; _id suppression; the exclusion default
//     never returns denied fields; keyset pagination through
//     data/mongo.ListCursor walks every page exactly once under a
//     projection that keeps the cursor fields, and is refused under one
//     that drops them.
//   - PostgreSQL: the quoted column list is valid SQL and returns exactly
//     the selected columns.
package projectionit
