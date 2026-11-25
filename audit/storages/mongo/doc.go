// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides a MongoDB-backed [audit.Storage] implementation.
// Events are persisted in a configurable collection with automatic index
// creation for efficient queries by time range, actor, resource, request ID
// and trace ID. [New] creates the indexes missing from an existing
// collection too.
// StoreBatch rejects a nil event before writing. Ordered insert errors can leave a stored prefix.
package mongo
