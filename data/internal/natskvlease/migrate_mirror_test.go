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

// mirrorEnv is an origin bucket with a memory-backed mirror of it.
type mirrorEnv struct {
	js     jetstream.JetStream
	origin jetstream.KeyValue
	revs   map[string]uint64
}

func newMirrorEnv(t *testing.T) *mirrorEnv {
	t.Helper()
	ctx := t.Context()
	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	origin, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "orig", TTL: time.Hour, Storage: jetstream.FileStorage})
	require.NoError(t, err)
	revs := map[string]uint64{}
	for _, kv := range [][2]string{{"a", "1"}, {"b", "1"}, {"a", "2"}, {"c", "1"}} {
		rev, err := origin.Put(ctx, kv[0], []byte(kv[1]))
		require.NoError(t, err)
		revs[kv[0]] = rev
	}
	require.NoError(t, origin.Delete(ctx, "c"))
	delete(revs, "c")

	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "mir",
		TTL:     time.Hour,
		Storage: jetstream.MemoryStorage,
		Mirror:  &jetstream.StreamSource{Name: "orig"},
	})
	require.NoError(t, err)
	e := &mirrorEnv{js: js, origin: origin, revs: revs}
	e.requireSynced(t)
	return e
}

func (e *mirrorEnv) target() BucketConfig {
	return BucketConfig{Bucket: "mir", TTL: time.Hour, Storage: jetstream.FileStorage}
}

// requireSynced waits until the mirror stream holds every origin key with the
// origin's value at the origin's revision. It reads the mirror stream itself:
// a mirror keeps the origin's subjects and sequence numbers.
func (e *mirrorEnv) requireSynced(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		mir, err := e.js.Stream(t.Context(), kvStreamName("mir"))
		if err != nil {
			return false
		}
		for key, rev := range e.revs {
			want, err := e.origin.Get(t.Context(), key)
			if err != nil {
				return false
			}
			got, err := mir.GetLastMsgForSubject(t.Context(), kvSubjectPrefix("orig")+key)
			if err != nil || got.Sequence != rev || string(got.Data) != string(want.Value()) {
				return false
			}
		}
		return true
	}, 5*time.Second, 50*time.Millisecond, "the mirror must hold the origin's entries at the origin's revisions")
}

func (e *mirrorEnv) requireMigrated(t *testing.T) {
	t.Helper()
	ctx := t.Context()

	s, err := e.js.Stream(ctx, kvStreamName("mir"))
	require.NoError(t, err)
	cfg := s.CachedInfo().Config
	require.Equal(t, jetstream.FileStorage, cfg.Storage)
	require.NotNil(t, cfg.Mirror)
	require.Equal(t, kvStreamName("orig"), cfg.Mirror.Name)
	require.Empty(t, cfg.Subjects)
	_, err = e.js.Stream(ctx, markerStreamName("mir"))
	require.ErrorIs(t, err, jetstream.ErrStreamNotFound)

	e.requireSynced(t)
	maxRev := uint64(0)
	for _, rev := range e.revs {
		maxRev = max(maxRev, rev)
	}
	rev, err := e.origin.Put(ctx, "d", []byte("1"))
	require.NoError(t, err)
	require.Greater(t, rev, maxRev)
	e.revs["d"] = rev
	e.requireSynced(t)
}

// TestMigrateBucketStorage_Mirror moves a memory mirror to file storage: it is
// recreated as the same mirror and re-synced from its origin, keeping the
// origin's revisions.
func TestMigrateBucketStorage_Mirror(t *testing.T) {
	t.Parallel()

	e := newMirrorEnv(t)
	require.NoError(t, NewKVHelper(e.js, nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}))
	e.requireMigrated(t)
}

// TestMigrateBucketStorage_MirrorResume resumes a mirror migration interrupted
// after the old mirror was deleted.
func TestMigrateBucketStorage_MirrorResume(t *testing.T) {
	t.Parallel()

	e := newMirrorEnv(t)
	failing := NewKVHelper(e.js, nil)
	failing.failpoint = func(s string) error {
		if s == "after:source_deleted" {
			return errInjected
		}
		return nil
	}
	require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)
	_, err := e.js.Stream(t.Context(), kvStreamName("mir"))
	require.ErrorIs(t, err, jetstream.ErrStreamNotFound)

	require.NoError(t, NewKVHelper(e.js, nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true}))
	e.requireMigrated(t)
}

// TestMigrateBucketStorage_MirrorKeepsMirrorDirect pins that a mirror is
// recreated from its own configuration. With a local origin the server derives
// MirrorDirect from the origin; with an origin it cannot see — another domain
// or account, here simply absent — the configured MirrorDirect=false stands
// and must survive the migration.
func TestMigrateBucketStorage_MirrorKeepsMirrorDirect(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:              kvStreamName("ext"),
		Mirror:            &jetstream.StreamSource{Name: kvStreamName("remote")},
		MirrorDirect:      false,
		MaxMsgsPerSubject: 1,
		MaxAge:            time.Hour,
		Storage:           jetstream.MemoryStorage,
	})
	require.NoError(t, err)

	require.NoError(t, NewKVHelper(js, nil).MigrateBucketStorage(ctx,
		BucketConfig{Bucket: "ext", TTL: time.Hour, Storage: jetstream.FileStorage}, true, MigrationOptions{}))

	s, err := js.Stream(ctx, kvStreamName("ext"))
	require.NoError(t, err)
	cfg := s.CachedInfo().Config
	require.Equal(t, jetstream.FileStorage, cfg.Storage)
	require.False(t, cfg.MirrorDirect, "MirrorDirect must be kept")
	require.Equal(t, kvStreamName("remote"), cfg.Mirror.Name)
}
