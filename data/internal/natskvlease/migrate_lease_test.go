// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// blockAt returns a failpoint that parks the migration at step until release
// is closed, signalling reached when it gets there.
func blockAt(step string) (fp func(string) error, reached, release chan struct{}) {
	reached, release = make(chan struct{}), make(chan struct{})
	fired := false
	fp = func(s string) error {
		if s == step && !fired {
			fired = true
			close(reached)
			<-release
		}
		return nil
	}
	return fp, reached, release
}

func (e *migrationEnv) markerPhase(t *testing.T) string {
	t.Helper()
	s, err := e.js.Stream(t.Context(), markerStreamName(migBucket))
	require.NoError(t, err)
	return s.CachedInfo().Config.Metadata[metaPhase]
}

func (e *migrationEnv) leaseKV(t *testing.T) jetstream.KeyValue {
	t.Helper()
	kv, err := e.js.KeyValue(t.Context(), migrationLeaseBucket)
	require.NoError(t, err)
	return kv
}

// TestMigrateBucketStorage_ConcurrentMigrators pins that a second migrator,
// fresh or resuming, fails fast while the first one runs, without touching
// anything.
func TestMigrateBucketStorage_ConcurrentMigrators(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	last := e.seed(t)

	fp, reached, release := blockAt("after:sealed")
	done := make(chan error, 1)
	go func() { done <- e.helper(fp).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}) }()
	<-reached

	err := e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{})
	require.ErrorIs(t, err, ErrBucketMigrationLocked)
	err = e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true})
	require.ErrorIs(t, err, ErrBucketMigrationLocked)
	require.Equal(t, phaseSealed, e.markerPhase(t), "a locked-out migrator changed the migration")

	close(release)
	require.NoError(t, <-done)
	e.requireMigrated(t, last)
	require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}),
		"the lease must be released at the end")
}

// TestMigrateBucketStorage_CrashedMigrator pins that a lease left by a dead
// migrator blocks a Resume until it expires, after which the Resume completes.
func TestMigrateBucketStorage_CrashedMigrator(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	last := e.seed(t)

	failing := e.helper(func(s string) error {
		if s == "after:copied" {
			return errInjected
		}
		return nil
	})
	failing.leaseTTL = time.Second
	require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)

	// The crashed process never released its lease.
	_, err := e.leaseKV(t).Create(t.Context(), migBucket, []byte(`{"id":"dead"}`))
	require.NoError(t, err)

	err = e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true})
	require.ErrorIs(t, err, ErrBucketMigrationLocked)

	require.Eventually(t, func() bool {
		err := e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true})
		if err != nil {
			require.ErrorIs(t, err, ErrBucketMigrationLocked)
		}
		return err == nil
	}, 10*time.Second, 200*time.Millisecond, "the dead lease must expire")
	e.requireMigrated(t, last)
}

// TestMigrateBucketStorage_LostLeaseStops pins self-fencing: a migrator whose
// lease is taken while it is paused stops without advancing the migration.
func TestMigrateBucketStorage_LostLeaseStops(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	last := e.seed(t)

	fp, reached, release := blockAt("after:sealed")
	done := make(chan error, 1)
	go func() { done <- e.helper(fp).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}) }()
	<-reached

	_, err := e.leaseKV(t).Put(t.Context(), migBucket, []byte(`{"id":"thief"}`))
	require.NoError(t, err)
	close(release)
	require.ErrorIs(t, <-done, ErrBucketMigrationLocked)
	require.Equal(t, phaseSealed, e.markerPhase(t), "a migrator without its lease advanced the migration")

	require.NoError(t, e.leaseKV(t).Delete(t.Context(), migBucket))
	require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true}))
	e.requireMigrated(t, last)
}

// TestMigrateBucketStorage_Heartbeat pins that a migration outliving its lease
// TTL keeps the lease by renewing it.
func TestMigrateBucketStorage_Heartbeat(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	last := e.seed(t)

	h := e.helper(func(s string) error {
		if s == "after:copied" {
			time.Sleep(3 * time.Second)
		}
		return nil
	})
	h.leaseTTL = time.Second
	require.NoError(t, h.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}))
	e.requireMigrated(t, last)
}

// TestMigrateBucketStorage_ReleaseOnFailure pins that a failed run releases
// its lease, so it can be resumed at once.
func TestMigrateBucketStorage_ReleaseOnFailure(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	last := e.seed(t)
	failing := e.helper(func(s string) error {
		if s == "after:source_deleted" {
			return errInjected
		}
		return nil
	})
	require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)
	require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true}))
	e.requireMigrated(t, last)
}

func TestMigrateBucketStorage_LeaseStoreValidation(t *testing.T) {
	t.Parallel()

	leaseSubjects := []string{"$KV." + migrationLeaseBucket + ".>"}
	for name, cfg := range map[string]jetstream.StreamConfig{
		"foreign":     {MaxAge: time.Minute, Storage: jetstream.FileStorage, MaxMsgsPerSubject: 1},
		"no ttl":      {Storage: jetstream.FileStorage, MaxMsgsPerSubject: 1, Metadata: map[string]string{metaRole: roleLeases}},
		"memory":      {MaxAge: time.Minute, Storage: jetstream.MemoryStorage, MaxMsgsPerSubject: 1, Metadata: map[string]string{metaRole: roleLeases}},
		"discard old": {MaxAge: time.Minute, Storage: jetstream.FileStorage, MaxMsgsPerSubject: 1, MaxMsgs: 1, Discard: jetstream.DiscardOld, Metadata: map[string]string{metaRole: roleLeases}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newMigrationEnv(t)
			e.seed(t)
			cfg.Name = kvStreamName(migrationLeaseBucket)
			cfg.Subjects = leaseSubjects
			_, err := e.js.CreateStream(t.Context(), cfg)
			require.NoError(t, err)

			err = e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{})
			require.ErrorIs(t, err, ErrMigrationLeaseStore)
			require.False(t, e.sealed(t))
			require.False(t, e.streamExists(t, markerStreamName(migBucket)))
		})
	}
}

func TestMigrateBucketStorage_ReservedNames(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	for _, bucket := range []string{migrationLeaseBucket, migrationTemplatePrefix + "x"} {
		err := NewKVHelper(js, nil).MigrateBucketStorage(t.Context(),
			BucketConfig{Bucket: bucket, TTL: time.Hour, Storage: jetstream.FileStorage}, true, MigrationOptions{})
		require.ErrorIs(t, err, ErrMigrationUnsupportedBucket, bucket)
	}
}
