// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotencyconfig defines the idempotency key storage schema.
//
// Schemas carry yaml and default tags read by
// github.com/altessa-s/go-nomos/loader, expose Default* constructors
// where defaults exist, and implement Validate; component factories map them
// to generated options.
//
// Key types: [Config], [StorageConfig], [MongoStorageConfig], [SQLStorageConfig].
// SQL storage selects postgres or mysql (including MariaDB); its sqldb block
// configures the table, optional startup schema creation and expired-row sweep
// interval (default one minute, zero disables).
//
// # Purging
//
// The sqldb block exposes purgeInterval (default "1m", "0s" disables the background loop) and
// purgeBatchSize (default 1000). Manual PurgeExpired remains available with a zero interval. The
// background loop runs whenever purgeInterval is positive. The ttl setting governs expiry.
package idempotencyconfig
