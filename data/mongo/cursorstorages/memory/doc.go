// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides in-memory cursor storage for MongoDB pagination.
// Suitable for single-instance deployments with automatic TTL cleanup.
//
// Example:
//
//	storage := memory.New(time.Hour)
//	defer storage.Close()
//	result, _ := mongo.ListCursor(ctx, coll, mongo.WithListCursorStorage(storage))
package memory
