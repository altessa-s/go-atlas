// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides a MongoDB client wrapper with CSFLE encryption,
// transaction handling, and structured logging. Use it for database operations
// requiring encryption or simplified transaction management.
//
// # Cursor pagination
//
// ListCursor issues its continuation token in one of three modes, chosen by
// configuration and never by the shape of a presented token:
//
//   - Stateful (WithListCursorStorage): the token is the key of a record kept
//     server-side. The client can alter nothing, and the cursor is held to
//     the query it was issued for: the same collection, sort, cursor ID
//     field, filter and subject.
//   - Stateless signed (WithListCursorSigner): the token carries the same
//     bindings under the signature of a go-aip pagetoken codec, the one
//     keyset.FindPage uses. It gives the guarantees of stateful mode without
//     a storage; a token that does not verify, an unsigned one included,
//     fails with ErrInvalidCursor.
//   - Stateless unsigned (the default): the token is base64 JSON whose
//     checksum is not a signature. It must match the requested sort, cursor
//     ID field and filter, and its sort value is restricted to BSON scalars,
//     but the client can choose its own continuation position within that
//     query and reads the cursor ID and sort value of the last item of a page.
//
// Stateful storage snapshots metadata and returns owned copies. [CursorDeleter]
// is the optional capability for explicit cleanup.
//
// A subject (WithListCursorSubject) is matched exactly in the first two modes.
// The third cannot enforce one, so ListCursor given a subject without a
// storage or a signer fails with ErrCursorSubjectNotEnforced. When the sort
// field is hidden from the client, use storage or a signer that encrypts
// (pagetoken.WithEncryption).
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
// mongomigrations.RegisterMigration registers a versioned migration that Connect applies
// under the database-wide migration lock, in the mongo-migrate format. [WithMigrationRegistry] selects a client-specific set of steps.
// mongoindexes.Ensure creates a collection's indexes idempotently, optionally
// dropping obsolete ones and recreating those whose specification changed.
//
// # Write guards
//
// TouchGuard increments a storage-only counter on the document a filter
// matches (optionally upserting it). Inside a transaction this serializes
// conflicting writers through that document: a concurrent transaction that
// writes it — another guard, a delete — gets a write conflict.
//
// # Package layout
//
// Independent APIs live in explain, indexes, migrations and keyset.
// ConvertToNewDocument and ConvertToUpdateDocument use the MongoDB behavior
// translators for BSON layout and supply the store's CSFLE field transformer.
// The pagination APIs share query validation and aggregation rendering.
//
// # Usage
//
//	client, err := mongo.New("myapp",
//	    mongo.WithClientOptions(options.Client().ApplyURI(uri)),
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
