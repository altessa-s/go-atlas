// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package testhelpers provides shared test fixtures for the
// data/mongo subtree. It is internal to data/mongo and its
// subpackages (cursor_storages/*, kms/*, factory/*).
package testhelpers

import (
	"time"

	"github.com/altessa-s/go-atlas/data/mongo"
)

// SampleCursorMetadata returns a deterministic [mongo.CursorMetadata]
// fixture suitable for cursor-storage round-trip tests. The CursorId,
// Sort, CursorIdField, and FilterHash values are stable across calls;
// CreatedAt is set to [time.Now] so that TTL-aware tests can reason
// about freshness.
func SampleCursorMetadata() *mongo.CursorMetadata {
	return &mongo.CursorMetadata{
		CursorId:      "507f1f77bcf86cd799439011",
		Sort:          "dGVzdA==",
		CursorIdField: "_id",
		FilterHash:    "abc123",
		CreatedAt:     time.Now(),
	}
}
