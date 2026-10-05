// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
)

// MigrationOptions controls a [MigrateBucketStorage] run; see
// [MigrationOptions.Resume].
type MigrationOptions = natskvlease.MigrationOptions

// ErrBucketMigrationInProgress is returned by New while a storage migration of
// the bucket is unfinished, and by MigrateBucketStorage when it finds one
// without [MigrationOptions.Resume].
var ErrBucketMigrationInProgress = natskvlease.ErrBucketMigrationInProgress

// ErrBucketMigrationConflict is returned by MigrateBucketStorage when the
// bucket or the migration state changed unexpectedly; nothing is deleted.
var ErrBucketMigrationConflict = natskvlease.ErrBucketMigrationConflict

// ErrMigrationUnsupportedContext is returned by MigrateBucketStorage for a
// JetStream context with a domain or a custom API prefix.
var ErrMigrationUnsupportedContext = natskvlease.ErrMigrationUnsupportedContext

// ErrMigrationUnsupportedBucket is returned by MigrateBucketStorage for a
// bucket with sources, republishing, a subject transform or a placement, or a
// reserved bucket name. Mirrors are supported.
var ErrMigrationUnsupportedBucket = natskvlease.ErrMigrationUnsupportedBucket

// ErrBucketMigrationLocked is returned by MigrateBucketStorage while another
// migration of the bucket holds its lease, or when this one lost its lease.
var ErrBucketMigrationLocked = natskvlease.ErrBucketMigrationLocked

// ErrMigrationLeaseStore is returned by MigrateBucketStorage when the lease
// store bucket kvmigrate_leases exists but is not usable as one.
var ErrMigrationLeaseStore = natskvlease.ErrMigrationLeaseStore

// MigrateBucketStorage moves the existing bucket to the storage configured by
// the same opts as New ([WithStorage]; memory by default), keeping revisions monotonic: the migrated bucket's
// first revision is above the old bucket's last one, so fencing tokens keep
// growing. Its keys are not copied: with every user stopped no lease is held,
// and copying one would revive it.
//
// Stop every process using the bucket first. A second migration of the bucket
// fails with [ErrBucketMigrationLocked] while one runs. If a run fails, fix the
// cause, confirm it has exited and rerun with MigrationOptions{Resume: true}; until the migration completes, New fails
// with [ErrBucketMigrationInProgress]. A missing bucket, or one already on the
// configured storage, is left alone. See the package README for the procedure.
func MigrateBucketStorage(ctx context.Context, client *nats.Conn, mopts MigrationOptions, opts ...Option) error {
	js, err := jetstream.New(client)
	if err != nil {
		return err
	}
	return natskvlease.NewKVHelper(js, nil).MigrateBucketStorage(ctx, bucketConfig(newOptions(opts...)), false, mopts)
}
