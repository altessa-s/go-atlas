// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package brokerconfig defines the message broker schema with its outbox and
// in-progress tracking blocks.
//
// Schemas carry yaml and default tags read by
// github.com/altessa-s/go-nomos/loader, expose Default* constructors
// where defaults exist, and implement Validate; component factories map them
// to generated options.
//
// Key types: [Provider], [Config], [InProgressMetrics], [InProgress],
// [Outbox], [OutboxStorageConfig], [OutboxWatch].
//
// The outbox block selects its storage (mongo or sqldb) and how the
// dispatcher is woken beyond its schedule (polling, changeStream or notify);
// [Outbox.Validate] rejects a watch mode the storage cannot serve with
// [ErrUnsupportedOutboxWatch].
package brokerconfig
