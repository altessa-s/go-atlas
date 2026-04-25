// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package outboxstore provides MongoDB implementation of outbox.Store.
// Persists outbox events with automatic index creation and status tracking.
//
// Example:
//
//	store, _ := outboxstore.New(db,
//	    outboxstore.WithCollectionName("custom_outbox"),
//	)
//	ob := outbox.New(store, handler)
package outboxstore
