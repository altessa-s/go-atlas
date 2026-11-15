// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides a MongoDB-backed [audit.Storage] implementation.
// Events are persisted in a configurable collection with automatic index
// creation for efficient queries by actor, resource type, and time range.
package mongo
