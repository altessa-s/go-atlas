// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package natskvlease provides common utilities for NATS JetStream KeyValue-based
// distributed resource management, including distributed locks and leader election.
//
// This package extracts shared functionality to eliminate code duplication between
// dlock (distributed locks) and leadelect (leader election) packages:
//
//   - Retry logic with exponential backoff for transient network errors
//   - KeyValue bucket creation and management
//   - Lease-based resource management with camping loops that retry a failed
//     renewal for as long as the lease may still be valid
//   - Storage migration of an existing bucket (KVHelper.MigrateBucketStorage):
//     the bucket is sealed, copied into a marker stream and recreated on the
//     target storage with its first revision above the old last one, so
//     revision-based fencing tokens stay monotonic; the marker makes an
//     interrupted migration detectable and resumable, a lease in the
//     kvmigrate_leases bucket keeps a second migrator out, and a mirror is
//     recreated as the same mirror and re-synced from its origin
//
// The package is designed as an internal utility and should not be imported
// by external packages directly.
package natskvlease
