// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// White-box: the storage migration's failpoint and clock seams are unexported.
package natskvlease

import (
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

var errInjected = errors.New("injected failure")

const migBucket = "mig"

// migrationEnv is an embedded server with a memory-backed source bucket that
// supports per-key TTL, as the idempotency storage creates it.
type migrationEnv struct {
	nc *nats.Conn
	js jetstream.JetStream
	kv jetstream.KeyValue // a handle bound before the migration, as a running user would hold
}

func newMigrationEnv(t *testing.T) *migrationEnv {
	t.Helper()
	ns := testhelpers.StartNATSServer(t)
	nc, js := testhelpers.ConnectJetStream(t, ns)
	kv, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{
		Bucket:         migBucket,
		TTL:            time.Hour,
		Storage:        jetstream.MemoryStorage,
		LimitMarkerTTL: time.Hour,
	})
	require.NoError(t, err)
	return &migrationEnv{nc: nc, js: js, kv: kv}
}

// target is the migration target: the source's config on file storage.
func (e *migrationEnv) target() BucketConfig {
	return BucketConfig{Bucket: migBucket, TTL: time.Hour, Storage: jetstream.FileStorage, LimitMarkerTTL: time.Hour}
}

func (e *migrationEnv) helper(failpoint func(string) error) *KVHelper {
	h := NewKVHelper(e.js, nil)
	h.failpoint = failpoint
	return h
}

// seed writes plain keys a, b, c (a rewritten a few times), a per-key-TTL key
// and a deleted key, and returns the source's last revision.
func (e *migrationEnv) seed(t *testing.T) uint64 {
	t.Helper()
	ctx := t.Context()
	for i := range 5 {
		_, err := e.kv.Put(ctx, "a", []byte("a"+strconv.Itoa(i)))
		require.NoError(t, err)
	}
	for _, k := range []string{"b", "c"} {
		_, err := e.kv.Put(ctx, k, []byte(k))
		require.NoError(t, err)
	}
	_, err := e.kv.Create(ctx, "lock", []byte("held"), jetstream.KeyTTL(30*time.Second))
	require.NoError(t, err)
	_, err = e.kv.Put(ctx, "gone", []byte("x"))
	require.NoError(t, err)
	require.NoError(t, e.kv.Delete(ctx, "gone"))
	return e.lastSeq(t, kvStreamName(migBucket))
}

func (e *migrationEnv) lastSeq(t *testing.T, stream string) uint64 {
	t.Helper()
	s, err := e.js.Stream(t.Context(), stream)
	require.NoError(t, err)
	info, err := s.Info(t.Context())
	require.NoError(t, err)
	return info.State.LastSeq
}

func (e *migrationEnv) streamExists(t *testing.T, name string) bool {
	t.Helper()
	_, err := e.js.Stream(t.Context(), name)
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		return false
	}
	require.NoError(t, err)
	return true
}

func (e *migrationEnv) sealed(t *testing.T) bool {
	t.Helper()
	s, err := e.js.Stream(t.Context(), kvStreamName(migBucket))
	require.NoError(t, err)
	return s.CachedInfo().Config.Sealed
}

// requireMigrated checks the end state of a completed migration of a seeded
// bucket: file storage, entries back with revisions above the source's last,
// the per-key TTL kept, the deleted key gone and the marker removed.
func (e *migrationEnv) requireMigrated(t *testing.T, lastSeq uint64) {
	t.Helper()
	ctx := t.Context()

	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, e.js, migBucket))
	require.False(t, e.streamExists(t, markerStreamName(migBucket)), "the marker must be removed")

	kv, err := e.js.KeyValue(ctx, migBucket)
	require.NoError(t, err)
	for key, want := range map[string]string{"a": "a4", "b": "b", "c": "c", "lock": "held"} {
		entry, err := kv.Get(ctx, key)
		require.NoError(t, err, key)
		require.Equal(t, want, string(entry.Value()), key)
		require.Greater(t, entry.Revision(), lastSeq, "revision of %s must stay above the source's", key)
	}
	_, err = kv.Get(ctx, "gone")
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound)

	stream, err := e.js.Stream(ctx, kvStreamName(migBucket))
	require.NoError(t, err)
	lock, err := stream.GetLastMsgForSubject(ctx, kvSubjectPrefix(migBucket)+"lock")
	require.NoError(t, err)
	ttl, err := parseMessageTTL(lock.Header.Get(msgTTLHeader))
	require.NoError(t, err)
	require.LessOrEqual(t, ttl, 30*time.Second)
	require.Greater(t, ttl, 20*time.Second, "the per-key TTL must be carried over")
	plain, err := stream.GetLastMsgForSubject(ctx, kvSubjectPrefix(migBucket)+"a")
	require.NoError(t, err)
	require.Empty(t, plain.Header.Get(msgTTLHeader), "a plain entry must stay plain")

	rev, err := kv.Put(ctx, "after", []byte("x"))
	require.NoError(t, err)
	require.Greater(t, rev, lastSeq)

	_, err = NewKVHelper(e.js, nil).GetOrCreateBucket(ctx, BucketConfig{
		Bucket: migBucket, TTL: time.Hour, Storage: jetstream.FileStorage, LimitMarkerTTL: time.Hour, StrictStorage: true,
	})
	require.NoError(t, err)
}

func TestMigrateBucketStorage(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	last := e.seed(t)

	require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}))
	e.requireMigrated(t, last)
}

func TestMigrateBucketStorage_NothingToDo(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	cfg := e.target()

	cfg.Bucket = "missing"
	require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), cfg, true, MigrationOptions{}))
	require.False(t, e.streamExists(t, kvStreamName("missing")))

	cfg = e.target()
	cfg.Storage = jetstream.MemoryStorage
	require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), cfg, true, MigrationOptions{}))
	require.False(t, e.sealed(t))
	require.False(t, e.streamExists(t, markerStreamName(migBucket)))
}

// TestMigrateBucketStorage_ResumeAfterFailure interrupts the migration after
// every step and checks that a resumed run converges to the same end state
// and that constructors refuse the bucket in between.
func TestMigrateBucketStorage_ResumeAfterFailure(t *testing.T) {
	t.Parallel()

	for _, step := range []string{
		"before-seal", "sealed", "after:sealed", "inventory", "copied:b", "after:copied", "after:source_deleted",
		"after:replacement_created", "restore:b", "after:restored",
	} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()

			e := newMigrationEnv(t)
			last := e.seed(t)

			var once sync.Once
			failing := e.helper(func(s string) error {
				var err error
				if s == step {
					once.Do(func() { err = errInjected })
				}
				return err
			})
			require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)

			_, err := NewKVHelper(e.js, nil).GetOrCreateBucket(t.Context(), e.target())
			require.ErrorIs(t, err, ErrBucketMigrationInProgress)
			err = e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{})
			require.ErrorIs(t, err, ErrBucketMigrationInProgress, "a run without Resume must not continue a migration")

			require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true}))
			e.requireMigrated(t, last)
		})
	}
}

// TestMigrateBucketStorage_PartialRestoreIsKept resumes after some entries
// were restored: the replacement must not be deleted and restored entries must
// not be written again.
func TestMigrateBucketStorage_PartialRestoreIsKept(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	last := e.seed(t)

	restored := 0
	failing := e.helper(func(s string) error {
		if len(s) > 8 && s[:8] == "restore:" {
			if restored == 2 {
				return errInjected
			}
			restored++
		}
		return nil
	})
	require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)

	kv, err := e.js.KeyValue(t.Context(), migBucket)
	require.NoError(t, err)
	before := map[string]uint64{}
	keys, err := kv.Keys(t.Context())
	require.NoError(t, err)
	require.Len(t, keys, 2)
	for _, k := range keys {
		entry, err := kv.Get(t.Context(), k)
		require.NoError(t, err)
		before[k] = entry.Revision()
	}

	require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true}))
	e.requireMigrated(t, last)
	for k, rev := range before {
		entry, err := kv.Get(t.Context(), k)
		require.NoError(t, err)
		require.Equal(t, rev, entry.Revision(), "restored key %s was written again", k)
	}
}

// TestMigrateBucketStorage_StrayWrites pins what a user that kept running sees:
// writes into the replacement get revisions above the source's, and the
// restore neither overwrites them nor resurrects a key they deleted.
func TestMigrateBucketStorage_StrayWrites(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	last := e.seed(t)
	ctx := t.Context()

	failing := e.helper(func(s string) error {
		if s == "after:replacement_created" {
			return errInjected
		}
		return nil
	})
	require.ErrorIs(t, failing.MigrateBucketStorage(ctx, e.target(), true, MigrationOptions{}), errInjected)

	rev, err := e.kv.Put(ctx, "a", []byte("stray"))
	require.NoError(t, err)
	require.Greater(t, rev, last, "a stray write must get a revision above the source's")
	require.NoError(t, e.kv.Delete(ctx, "b"))

	require.NoError(t, e.helper(nil).MigrateBucketStorage(ctx, e.target(), true, MigrationOptions{Resume: true}))

	entry, err := e.kv.Get(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, "stray", string(entry.Value()), "the restore overwrote a stray write")
	_, err = e.kv.Get(ctx, "b")
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound, "the restore resurrected a deleted key")
}

// TestMigrateBucketStorage_ExpiryDuringRestore pins that the remaining
// lifetime is recomputed before every publish attempt: an entry whose deadline
// passes while a publish is retried is skipped, not revived.
func TestMigrateBucketStorage_ExpiryDuringRestore(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	e.seed(t)

	var mu sync.Mutex
	offset := time.Duration(0)
	h := e.helper(func(s string) error {
		if s != "restore:lock" {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		if offset == 0 {
			offset = 31 * time.Second
			return nats.ErrNoResponders
		}
		return nil
	})
	h.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return time.Now().Add(offset)
	}

	require.NoError(t, h.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}))

	kv, err := e.js.KeyValue(t.Context(), migBucket)
	require.NoError(t, err)
	_, err = kv.Get(t.Context(), "lock")
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound, "an entry past its deadline was restored")
	_, err = kv.Get(t.Context(), "a")
	require.NoError(t, err)
}

func TestMigrateBucketStorage_TTL(t *testing.T) {
	t.Parallel()

	t.Run("mismatch is rejected before anything is touched", func(t *testing.T) {
		t.Parallel()
		e := newMigrationEnv(t)
		e.seed(t)
		cfg := e.target()
		cfg.TTL = 2 * time.Hour

		err := e.helper(nil).MigrateBucketStorage(t.Context(), cfg, true, MigrationOptions{})
		require.ErrorIs(t, err, ErrBucketTTLMismatch)
		require.False(t, e.sealed(t))
		require.False(t, e.streamExists(t, markerStreamName(migBucket)))
	})

	t.Run("longer TTL keeps per-key deadlines", func(t *testing.T) {
		t.Parallel()
		e := newMigrationEnv(t)
		last := e.seed(t)
		cfg := e.target()
		cfg.TTL = 2 * time.Hour
		cfg.MigrateTTL = true

		require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), cfg, true, MigrationOptions{}))
		require.Equal(t, 2*time.Hour, testhelpers.KVBucketTTL(t, e.js, migBucket))

		stream, err := e.js.Stream(t.Context(), kvStreamName(migBucket))
		require.NoError(t, err)
		lock, err := stream.GetLastMsgForSubject(t.Context(), kvSubjectPrefix(migBucket)+"lock")
		require.NoError(t, err)
		ttl, err := parseMessageTTL(lock.Header.Get(msgTTLHeader))
		require.NoError(t, err)
		require.LessOrEqual(t, ttl, 30*time.Second, "an in-progress lock keeps its own deadline")
		plain, err := stream.GetLastMsgForSubject(t.Context(), kvSubjectPrefix(migBucket)+"a")
		require.NoError(t, err)
		require.Empty(t, plain.Header.Get(msgTTLHeader), "a completed entry follows the bucket TTL")
		require.Greater(t, plain.Sequence, last)
	})

	t.Run("shorter TTL caps entries", func(t *testing.T) {
		t.Parallel()
		e := newMigrationEnv(t)
		e.seed(t)
		cfg := e.target()
		cfg.TTL = 30 * time.Minute
		cfg.MigrateTTL = true

		h := e.helper(nil)
		h.now = func() time.Time { return time.Now().Add(45 * time.Minute) }
		require.NoError(t, h.MigrateBucketStorage(t.Context(), cfg, true, MigrationOptions{}))

		kv, err := e.js.KeyValue(t.Context(), migBucket)
		require.NoError(t, err)
		_, err = kv.Get(t.Context(), "a")
		require.ErrorIs(t, err, jetstream.ErrKeyNotFound, "an entry past the new, shorter TTL must not be restored")
	})
}

// TestMigrateBucketStorage_MessageTTLForms pins the three per-message TTL
// states: an effective zero is a plain entry, "never" survives even past the
// bucket TTL and a shorter target TTL.
func TestMigrateBucketStorage_MessageTTLForms(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	ctx := t.Context()
	for key, ttl := range map[string]string{"zero": "00", "forever": "never"} {
		msg := nats.NewMsg(kvSubjectPrefix(migBucket) + key)
		msg.Data = []byte(key)
		msg.Header.Set(msgTTLHeader, ttl)
		_, err := e.js.PublishMsg(ctx, msg)
		require.NoError(t, err)
	}

	cfg := e.target()
	cfg.TTL = 30 * time.Minute
	cfg.MigrateTTL = true
	h := e.helper(nil)
	h.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	require.NoError(t, h.MigrateBucketStorage(ctx, cfg, true, MigrationOptions{}))

	stream, err := e.js.Stream(ctx, kvStreamName(migBucket))
	require.NoError(t, err)
	forever, err := stream.GetLastMsgForSubject(ctx, kvSubjectPrefix(migBucket)+"forever")
	require.NoError(t, err)
	require.Equal(t, msgTTLNever, forever.Header.Get(msgTTLHeader))
	_, err = stream.GetLastMsgForSubject(ctx, kvSubjectPrefix(migBucket)+"zero")
	require.ErrorIs(t, err, jetstream.ErrMsgNotFound, "a plain entry 2h old is past the 30m TTL")

	e2 := newMigrationEnv(t)
	msg := nats.NewMsg(kvSubjectPrefix(migBucket) + "zero")
	msg.Data = []byte("zero")
	msg.Header.Set(msgTTLHeader, "00")
	_, err = e2.js.PublishMsg(ctx, msg)
	require.NoError(t, err)
	require.NoError(t, e2.helper(nil).MigrateBucketStorage(ctx, e2.target(), true, MigrationOptions{}))
	stream2, err := e2.js.Stream(ctx, kvStreamName(migBucket))
	require.NoError(t, err)
	zero, err := stream2.GetLastMsgForSubject(ctx, kvSubjectPrefix(migBucket)+"zero")
	require.NoError(t, err, "an effective zero TTL is a live plain entry")
	require.Empty(t, zero.Header.Get(msgTTLHeader))
}

func TestMigrateBucketStorage_ResumeWithOtherTarget(t *testing.T) {
	t.Parallel()

	for name, change := range map[string]func(*BucketConfig, *bool){
		"ttl":     func(c *BucketConfig, _ *bool) { c.TTL = 2 * time.Hour; c.MigrateTTL = true },
		"storage": func(c *BucketConfig, _ *bool) { c.Storage = jetstream.MemoryStorage },
		"copy":    func(_ *BucketConfig, copyEntries *bool) { *copyEntries = false },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newMigrationEnv(t)
			e.seed(t)
			failing := e.helper(func(s string) error {
				if s == "before-seal" {
					return errInjected
				}
				return nil
			})
			require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)

			cfg, copyEntries := e.target(), true
			change(&cfg, &copyEntries)
			err := e.helper(nil).MigrateBucketStorage(t.Context(), cfg, copyEntries, MigrationOptions{Resume: true})
			require.ErrorIs(t, err, ErrBucketMigrationConflict)
			require.False(t, e.sealed(t), "a rejected resume touched the source")
		})
	}
}

// TestMigrateBucketStorage_SourceReplaced pins that a bucket recreated by
// someone else after the marker was created is never sealed or deleted.
func TestMigrateBucketStorage_SourceReplaced(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	e.seed(t)
	failing := e.helper(func(s string) error {
		if s == "before-seal" {
			return errInjected
		}
		return nil
	})
	require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)

	require.NoError(t, e.js.DeleteStream(t.Context(), kvStreamName(migBucket)))
	time.Sleep(10 * time.Millisecond) // a distinct creation time
	other := testhelpers.CreateNATSKV(t, e.js, migBucket, time.Hour)
	_, err := other.Put(t.Context(), "foreign", []byte("x"))
	require.NoError(t, err)

	err = e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true})
	require.ErrorIs(t, err, ErrBucketMigrationConflict)
	require.False(t, e.sealed(t))
	_, err = other.Get(t.Context(), "foreign")
	require.NoError(t, err)
}

func TestMigrateBucketStorage_ForeignReplacement(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	e.seed(t)
	failing := e.helper(func(s string) error {
		if s == "after:source_deleted" {
			return errInjected
		}
		return nil
	})
	require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)

	foreign := testhelpers.CreateNATSKV(t, e.js, migBucket, time.Hour)
	_, err := foreign.Put(t.Context(), "foreign", []byte("x"))
	require.NoError(t, err)

	err = e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true})
	require.ErrorIs(t, err, ErrBucketMigrationConflict)
	_, err = foreign.Get(t.Context(), "foreign")
	require.NoError(t, err, "a foreign bucket must be left alone")
}

func TestMigrateBucketStorage_Rejections(t *testing.T) {
	t.Parallel()

	t.Run("domain context", func(t *testing.T) {
		t.Parallel()
		e := newMigrationEnv(t)
		js, err := jetstream.NewWithDomain(e.nc, "elsewhere")
		require.NoError(t, err)
		err = NewKVHelper(js, nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{})
		require.ErrorIs(t, err, ErrMigrationUnsupportedContext)
		require.False(t, e.streamExists(t, markerStreamName(migBucket)))
	})

	t.Run("sealed without marker", func(t *testing.T) {
		t.Parallel()
		e := newMigrationEnv(t)
		s, err := e.js.Stream(t.Context(), kvStreamName(migBucket))
		require.NoError(t, err)
		cfg := s.CachedInfo().Config
		cfg.Sealed = true
		_, err = e.js.UpdateStream(t.Context(), cfg)
		require.NoError(t, err)

		err = e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{})
		require.ErrorIs(t, err, ErrBucketMigrationConflict)
	})

	for name, kvCfg := range map[string]jetstream.KeyValueConfig{
		"sources":   {Sources: []*jetstream.StreamSource{{Name: "upstream"}}},
		"republish": {RePublish: &jetstream.RePublish{Source: ">", Destination: "republished.>"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ns := testhelpers.StartNATSServer(t)
			_, js := testhelpers.ConnectJetStream(t, ns)
			testhelpers.CreateNATSKV(t, js, "upstream", time.Hour)
			kvCfg.Bucket = "topo"
			kvCfg.TTL = time.Hour
			kvCfg.Storage = jetstream.MemoryStorage
			_, err := js.CreateKeyValue(t.Context(), kvCfg)
			require.NoError(t, err)

			err = NewKVHelper(js, nil).MigrateBucketStorage(t.Context(),
				BucketConfig{Bucket: "topo", TTL: time.Hour, Storage: jetstream.FileStorage}, true, MigrationOptions{})
			require.ErrorIs(t, err, ErrMigrationUnsupportedBucket)
			s, err := js.Stream(t.Context(), kvStreamName("topo"))
			require.NoError(t, err)
			require.False(t, s.CachedInfo().Config.Sealed)
			_, err = js.Stream(t.Context(), markerStreamName("topo"))
			require.ErrorIs(t, err, jetstream.ErrStreamNotFound)
		})
	}

	for name, invalid := range map[string]func(*BucketConfig){
		"invalid replicas":         func(c *BucketConfig) { c.Replicas = -1 },
		"invalid limit marker TTL": func(c *BucketConfig) { c.LimitMarkerTTL = 100 * time.Millisecond },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newMigrationEnv(t)
			last := e.seed(t)
			cfg := e.target()
			invalid(&cfg)

			err := e.helper(nil).MigrateBucketStorage(t.Context(), cfg, true, MigrationOptions{})
			require.Error(t, err)
			require.False(t, e.sealed(t))
			require.False(t, e.streamExists(t, markerStreamName(migBucket)))
			_, err = e.kv.Put(t.Context(), "writable", []byte("x"))
			require.NoError(t, err, "a rejected migration left the source unwritable")

			require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}))
			e.requireMigrated(t, last)
		})
	}
}

// TestMigrateBucketStorage_Templates pins the template cleanup: an orphan
// template of a crashed earlier run is deleted, a foreign stream named like a
// template is not, and neither is a completed replacement.
func TestMigrateBucketStorage_Templates(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	last := e.seed(t)
	ctx := t.Context()
	require.NoError(t, e.helper(nil).MigrateBucketStorage(ctx, e.target(), true, MigrationOptions{}))
	e.requireMigrated(t, last)

	_, err := e.js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:   migrationTemplatePrefix + "deadbeef",
		Storage:  jetstream.MemoryStorage,
		Metadata: map[string]string{metaID: "deadbeef", metaRole: roleTemplate, metaBucket: migBucket},
	})
	require.NoError(t, err)
	foreign, err := e.js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:   migrationTemplatePrefix + "cafe",
		Storage:  jetstream.MemoryStorage,
		Metadata: map[string]string{metaID: "cafe", metaBucket: migBucket},
	})
	require.NoError(t, err)
	_, err = foreign.Put(ctx, "keep", []byte("x"))
	require.NoError(t, err)

	back := e.target()
	back.Storage = jetstream.MemoryStorage
	require.NoError(t, e.helper(nil).MigrateBucketStorage(ctx, back, true, MigrationOptions{}))

	require.False(t, e.streamExists(t, kvStreamName(migrationTemplatePrefix+"deadbeef")), "the orphan template must be cleaned up")
	_, err = foreign.Get(ctx, "keep")
	require.NoError(t, err, "a foreign stream named like a template must be left alone")
	require.Equal(t, jetstream.MemoryStorage, testhelpers.KVBucketStorage(t, e.js, migBucket))
	kv, err := e.js.KeyValue(ctx, migBucket)
	require.NoError(t, err)
	entry, err := kv.Get(ctx, "after")
	require.NoError(t, err, "data written after the first migration must survive the second")
	require.Greater(t, entry.Revision(), last)
}

func TestParseMessageTTL(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]time.Duration{
		"never": -1, "NEVER": -1, "30s": 30 * time.Second, "1.9s": time.Second, "00": 0, "5": 5 * time.Second,
	} {
		got, err := parseMessageTTL(raw)
		require.NoError(t, err, raw)
		require.Equal(t, want, got, raw)
	}
	for _, raw := range []string{"500ms", "abc", "-5"} {
		_, err := parseMessageTTL(raw)
		require.Error(t, err, raw)
	}
}

// TestCopiedTTLHeader pins that a source entry's Nats-TTL survives the trip
// through the marker header with the value nats-server would apply.
func TestCopiedTTLHeader(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		raw    string
		header string
		want   time.Duration
	}{
		{raw: "", header: "", want: 0},
		{raw: "00", header: "", want: 0},
		{raw: "never", header: msgTTLNever, want: -1},
		{raw: "NEVER", header: msgTTLNever, want: -1},
		{raw: "30", header: "30", want: 30 * time.Second},
		{raw: "30s", header: "30", want: 30 * time.Second},
		{raw: "1m30s", header: "90", want: 90 * time.Second},
	} {
		header, err := copiedTTLHeader(tc.raw)
		require.NoError(t, err, tc.raw)
		require.Equal(t, tc.header, header, tc.raw)
		got, err := parseCopiedTTL(header)
		require.NoError(t, err, tc.raw)
		require.Equal(t, tc.want, got, tc.raw)
	}
	for _, raw := range []string{"500ms", "-1", "x"} {
		_, err := copiedTTLHeader(raw)
		require.Error(t, err, raw)
	}
	_, err := parseCopiedTTL("x")
	require.Error(t, err)
}

func TestRestoreLifetime(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_000_000, 0)
	unchanged := &migrationState{sourceTTL: time.Hour, target: migrationTarget{TTL: time.Hour}}

	for _, tc := range []struct {
		name     string
		st       *migrationState
		written  time.Time
		msgTTL   time.Duration
		perKey   bool
		wantTTL  string
		wantLive bool
	}{
		{"per-key fraction rounds up", unchanged, now.Add(-200 * time.Millisecond), time.Second, true, "1s", true},
		{"per-key effective TTL expired", unchanged, now.Add(-1100 * time.Millisecond), time.Second, true, "", false},
		{"per-key capped by unchanged bucket TTL", unchanged, now.Add(-59 * time.Minute), 2 * time.Hour, true, "60s", true},
		{"never", unchanged, now.Add(-3 * time.Hour), -1, true, "never", true},
		{"never without per-key support", unchanged, now.Add(-3 * time.Hour), -1, false, "", true},
		{"plain alive", unchanged, now.Add(-59 * time.Minute), 0, false, "", true},
		{"plain expired", unchanged, now.Add(-time.Hour), 0, false, "", false},
		{"plain without TTL", &migrationState{}, now.Add(-100 * time.Hour), 0, false, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ttl, live := restoreLifetime(tc.st, tc.written, tc.msgTTL, tc.perKey, now)
			require.Equal(t, tc.wantLive, live)
			require.Equal(t, tc.wantTTL, ttl)
		})
	}
}

// markerID reads the migration id from the bucket's marker.
func (e *migrationEnv) markerID(t *testing.T) string {
	t.Helper()
	s, err := e.js.Stream(t.Context(), markerStreamName(migBucket))
	require.NoError(t, err)
	return s.CachedInfo().Config.Metadata[metaID]
}

// TestMigrateBucketStorage_ReplacementBelowFloor pins that a replacement
// carrying the migration id but revisions at or below the source's last one —
// recreated by hand, say — is rejected instead of being restored into.
func TestMigrateBucketStorage_ReplacementBelowFloor(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	e.seed(t)
	ctx := t.Context()
	failing := e.helper(func(s string) error {
		if s == "after:source_deleted" {
			return errInjected
		}
		return nil
	})
	require.ErrorIs(t, failing.MigrateBucketStorage(ctx, e.target(), true, MigrationOptions{}), errInjected)

	kv, err := e.js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:         migBucket,
		TTL:            time.Hour,
		Storage:        jetstream.FileStorage,
		LimitMarkerTTL: time.Hour,
		Metadata:       map[string]string{metaID: e.markerID(t), metaRole: roleReplacement},
	})
	require.NoError(t, err)
	_, err = kv.Put(ctx, "low", []byte("x"))
	require.NoError(t, err)

	err = e.helper(nil).MigrateBucketStorage(ctx, e.target(), true, MigrationOptions{Resume: true})
	require.ErrorIs(t, err, ErrBucketMigrationConflict)
	require.True(t, e.streamExists(t, markerStreamName(migBucket)), "the marker must be kept")
}

// TestMigrateBucketStorage_ReplacementLostAfterRestore pins that the marker —
// the last record of the data and the revision floor — is only removed while
// the replacement is verifiably in place.
func TestMigrateBucketStorage_ReplacementLostAfterRestore(t *testing.T) {
	t.Parallel()

	for name, tamper := range map[string]func(*testing.T, *migrationEnv){
		"missing": func(t *testing.T, e *migrationEnv) {
			require.NoError(t, e.js.DeleteStream(t.Context(), kvStreamName(migBucket)))
		},
		"foreign": func(t *testing.T, e *migrationEnv) {
			require.NoError(t, e.js.DeleteStream(t.Context(), kvStreamName(migBucket)))
			testhelpers.CreateNATSKV(t, e.js, migBucket, time.Hour)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newMigrationEnv(t)
			e.seed(t)
			failing := e.helper(func(s string) error {
				if s == "after:restored" {
					return errInjected
				}
				return nil
			})
			require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)
			tamper(t, e)

			err := e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true})
			require.ErrorIs(t, err, ErrBucketMigrationConflict)
			require.True(t, e.streamExists(t, markerStreamName(migBucket)), "the marker must be kept")
		})
	}
}

// TestMigrateBucketStorage_LeaderServedReads pins that the sealed source has
// direct gets turned off, so the copy reads every entry from the stream leader
// and never from a replica that may lag behind.
func TestMigrateBucketStorage_LeaderServedReads(t *testing.T) {
	t.Parallel()

	e := newMigrationEnv(t)
	e.seed(t)
	s, err := e.js.Stream(t.Context(), kvStreamName(migBucket))
	require.NoError(t, err)
	require.True(t, s.CachedInfo().Config.AllowDirect, "KV buckets allow direct gets by default")

	failing := e.helper(func(step string) error {
		if step == "after:sealed" {
			return errInjected
		}
		return nil
	})
	require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)

	s, err = e.js.Stream(t.Context(), kvStreamName(migBucket))
	require.NoError(t, err)
	require.True(t, s.CachedInfo().Config.Sealed)
	require.False(t, s.CachedInfo().Config.AllowDirect, "the copy must not use direct gets")
}

// TestMigrateBucketStorage_LegacyMarker resumes markers written before the
// migration kind was recorded: they are standalone migrations.
func TestMigrateBucketStorage_LegacyMarker(t *testing.T) {
	t.Parallel()

	for _, step := range []string{"after:sealed", "after:source_deleted"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			e := newMigrationEnv(t)
			last := e.seed(t)
			failing := e.helper(func(s string) error {
				if s == step {
					return errInjected
				}
				return nil
			})
			require.ErrorIs(t, failing.MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{}), errInjected)

			s, err := e.js.Stream(t.Context(), markerStreamName(migBucket))
			require.NoError(t, err)
			cfg := s.CachedInfo().Config
			delete(cfg.Metadata, metaKind)
			_, err = e.js.UpdateStream(t.Context(), cfg)
			require.NoError(t, err)

			require.NoError(t, e.helper(nil).MigrateBucketStorage(t.Context(), e.target(), true, MigrationOptions{Resume: true}))
			e.requireMigrated(t, last)
		})
	}
}
