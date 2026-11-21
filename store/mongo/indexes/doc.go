// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package indexes creates and reconciles MongoDB collection indexes.
//
// # Usage
//
//	err := indexes.Ensure(ctx, collection, models)
//
// # Reconciliation
//
// Ensure validates model names, key documents and obsolete names before dropping any index.
// Existing equal specifications are left in place. Destructive replacement
// requires WithDropOnSpecMismatch; it uses a per-collection database lease.
package indexes
