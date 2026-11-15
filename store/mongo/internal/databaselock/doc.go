// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package databaselock provides MongoDB leases shared by migrations and index reconciliation.
//
// # Ownership
//
// Lease release checks the owner and uses a bounded cleanup context.
package databaselock
