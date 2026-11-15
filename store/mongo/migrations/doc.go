// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package migrations registers and runs MongoDB schema migrations under a database lease.
//
// # Ownership
//
// Lease release checks the owner and uses a bounded cleanup context.
//
// # Usage
//
//	err := migrations.Run(ctx, db)
package migrations
