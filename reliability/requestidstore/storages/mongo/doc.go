// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo persists AIP-155 reservations and replay outcomes for a requestid.Guard.
//
// Each operation checks ExpiresAt against the caller's now. The physical TTL assumes that
// elapsed backend time can exceed elapsed caller time by less than [DefaultCleanupGrace]
// or the margin configured with [WithCleanupGrace].
//
// # Usage
//
//	store, err := mongo.New(ctx, db, mongo.WithCollectionName("request_ids"))
//	if err != nil {
//		return err
//	}
//	guard, err := requestid.New(store)
//
// # Expiration
//
// The caller supplies the logical clock for every transition. A separate
// server-relative deadline lets the MongoDB TTL monitor reclaim old documents.
package mongo
