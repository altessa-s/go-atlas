// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides a MongoDB client wrapper with CSFLE encryption,
// transaction handling, and structured logging. Use it for database operations
// requiring encryption or simplified transaction management.
//
// # Cursor pagination
//
// ListCursor accepts only server-issued storage keys when cursor storage is
// configured. Stateless cursors must match the requested sort, cursor ID field,
// and filter; their sort values are restricted to BSON scalars. Stateless
// checksums are not signatures: use storage with subject binding or signed
// keyset tokens when callers must not choose their own continuation position.
//
// # Transactions and upserts
//
// WithTransaction joins a transaction of the same client that ctx already
// carries instead of starting an independent one; InTransaction reports
// whether ctx carries a running transaction. SetOnInsertFrom builds the
// $setOnInsert document of an upsert from a typed document, and
// UpsertRetryingDuplicate repeats an upsert that lost a first-insert race
// (never inside a transaction). Now and StoredTime give times at MongoDB
// precision (UTC, milliseconds).
//
// # Migrations and indexes
//
// RegisterMigration registers a versioned migration that Connect applies
// under the database-wide migration lock, in the mongo-migrate format.
// EnsureIndexes creates a collection's indexes idempotently, optionally
// dropping obsolete ones and recreating those whose specification changed.
//
// # Write guards
//
// TouchGuard increments a storage-only counter on the document a filter
// matches (optionally upserting it). Inside a transaction this serializes
// conflicting writers through that document: a concurrent transaction that
// writes it — another guard, a delete — gets a write conflict.
//
// # Usage
//
//	client, err := mongo.New("myapp",
//	    mongo.WithMongoClientOptions(options.Client().ApplyURI(uri)),
//	    mongo.WithLogger(logger),
//	)
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
//	if err := client.Connect(ctx); err != nil {
//	    return err
//	}
package mongo
