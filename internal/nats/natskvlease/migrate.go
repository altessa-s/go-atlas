// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// ErrBucketMigrationInProgress reports that a storage migration of the bucket
// has started and not finished: its marker stream exists. Constructors refuse
// the bucket until the migration completes; a migration run refuses to start
// unless [MigrationOptions.Resume] is set.
var ErrBucketMigrationInProgress = errors.New("KeyValue bucket storage migration in progress")

// ErrBucketMigrationConflict reports that the bucket or the migration state
// changed in a way the migration did not expect — the bucket was recreated or
// written by someone else, a resumed run asked for a different target, or a
// stream it would touch is not its own. Nothing is deleted when it is
// returned.
var ErrBucketMigrationConflict = errors.New("KeyValue bucket changed during storage migration")

// ErrMigrationUnsupportedContext reports a JetStream context with a domain or
// a custom API prefix. The migration publishes to raw stream subjects, which
// such a context would not route like its KeyValue API does.
var ErrMigrationUnsupportedContext = errors.New("storage migration requires a JetStream context without domain or API prefix")

// ErrMigrationUnsupportedBucket reports a bucket whose stream uses sources,
// republishing, a subject transform or a placement, or a reserved bucket
// name. Recreating a sourced bucket replays or loses source data, and the
// others cannot be recreated safely; mirrors are supported.
var ErrMigrationUnsupportedBucket = errors.New("storage migration supports only standalone KeyValue buckets")

// MigrationOptions controls a storage migration run.
type MigrationOptions struct {
	// Resume continues an interrupted migration of the bucket. Without it a
	// run that finds an unfinished migration fails with
	// ErrBucketMigrationInProgress, so a second migrator cannot join a running
	// one by accident. Set it only once the previous run is known to have
	// stopped: running two migrations of one bucket at once is unsupported.
	Resume bool
}

const (
	migrationMarkerPrefix   = "KVMIGRATE_"
	migrationSubjectPrefix  = "_kvmigrate."
	migrationTemplatePrefix = "kvmigrate_"

	metaPrefix         = "go-atlas.kvmigrate."
	metaID             = metaPrefix + "id"
	metaRole           = metaPrefix + "role"
	metaBucket         = metaPrefix + "bucket"
	metaPhase          = metaPrefix + "phase"
	metaSourceTTL      = metaPrefix + "source_ttl"
	metaSourceStorage  = metaPrefix + "source_storage"
	metaSourceCreated  = metaPrefix + "source_created"
	metaSourceLastSeq  = metaPrefix + "source_last_seq"
	metaTarget         = metaPrefix + "target"
	metaTargetStream   = metaPrefix + "target_stream"
	metaTemplateBucket = metaPrefix + "template_bucket"
	metaCopied         = metaPrefix + "copied"
	metaKind           = metaPrefix + "kind"

	kindStandalone = "standalone"
	kindMirror     = "mirror"

	roleMarker      = "marker"
	roleTemplate    = "template"
	roleReplacement = "replacement"

	phasePrepared           = "prepared"
	phaseSealed             = "sealed"
	phaseCopied             = "copied"
	phaseSourceDeleted      = "source_deleted"
	phaseReplacementCreated = "replacement_created"
	phaseRestored           = "restored"

	headerTime   = "Kv-Migrate-Time"
	headerMsgTTL = "Kv-Migrate-Msg-TTL"

	kvOperationHeader  = "KV-Operation"
	markerReasonHeader = "Nats-Marker-Reason"
	msgTTLHeader       = "Nats-TTL"
	msgTTLNever        = "never"

	restoreAttempts = 5
	restoreBackoff  = 100 * time.Millisecond
)

// migrationTarget is the normalized configuration a migration converges to,
// persisted in the marker so that a resumed run cannot change it.
type migrationTarget struct {
	Bucket         string                `json:"bucket"`
	Storage        jetstream.StorageType `json:"storage"`
	TTL            time.Duration         `json:"ttl"`
	NoTTL          bool                  `json:"no_ttl"`
	Replicas       int                   `json:"replicas"`
	Compression    bool                  `json:"compression"`
	LimitMarkerTTL time.Duration         `json:"limit_marker_ttl"`
	CopyEntries    bool                  `json:"copy_entries"`
}

// migrationState is the marker's metadata in typed form.
type migrationState struct {
	id            string
	phase         string
	sourceTTL     time.Duration
	sourceStorage jetstream.StorageType
	sourceCreated int64
	sourceLastSeq uint64
	target        migrationTarget
	targetStream  jetstream.StreamConfig
	copied        int
	kind          string

	// lease is this run's hold on the bucket; not persisted.
	lease *migrationLease
}

// MigrateBucketStorage moves the existing bucket cfg.Bucket to the storage
// type, and the rest of the configuration, in cfg, keeping its data and the
// monotonicity of its revisions. It is an operator action: every process using
// the bucket must be stopped, and only one migration of a bucket may run at a
// time.
//
// The bucket is sealed, its live entries are copied into a marker stream
// (KVMIGRATE_<bucket>) together with their timestamps and per-message TTLs,
// and the bucket is recreated with its first revision just above the old
// bucket's last one — so revision-based fencing tokens stay monotonic — before
// the entries are restored with their remaining lifetimes. The marker records
// the progress: an interrupted run leaves it in place, constructors then fail
// with ErrBucketMigrationInProgress, and a run with Resume completes it.
//
// copyEntries restores the bucket's entries; lease buckets pass false, as no
// lease is held while every user is stopped and copying one would revive it.
// A missing bucket, or one already on the target storage, is left alone.
func (h *KVHelper) MigrateBucketStorage(ctx context.Context, cfg BucketSpec, copyEntries bool, opts MigrationOptions) error {
	if err := h.checkMigrationContext(); err != nil {
		return err
	}
	if isReservedBucket(cfg.Bucket) {
		return fmt.Errorf("%w: bucket name %q is reserved for the migration itself", ErrMigrationUnsupportedBucket, cfg.Bucket)
	}

	lease, err := h.acquireLease(ctx, cfg.Bucket)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	lease.startHeartbeat(runCtx, cancel)
	defer lease.release(context.WithoutCancel(ctx))

	err = h.migrateLeased(runCtx, cfg, copyEntries, opts, lease)
	if cause := context.Cause(runCtx); cause != nil && errors.Is(cause, ErrBucketMigrationLocked) {
		return cause
	}
	return err
}

// migrateLeased runs a migration while this migrator holds the bucket's lease.
func (h *KVHelper) migrateLeased(ctx context.Context, cfg BucketSpec, copyEntries bool, opts MigrationOptions, lease *migrationLease) error {
	cfg = cfg.normalized()
	target := migrationTarget{
		Bucket:         cfg.Bucket,
		Storage:        cfg.Storage,
		TTL:            cfg.TTL,
		NoTTL:          cfg.NoTTL,
		Replicas:       cfg.Replicas,
		Compression:    cfg.Compression,
		LimitMarkerTTL: cfg.LimitMarkerTTL,
		CopyEntries:    copyEntries,
	}

	marker, err := h.streamInfo(ctx, markerStreamName(cfg.Bucket))
	if err != nil {
		return err
	}
	if marker != nil {
		if !opts.Resume {
			return fmt.Errorf("%w: bucket %q; rerun with Resume once the previous migration has stopped",
				ErrBucketMigrationInProgress, cfg.Bucket)
		}
		st, perr := parseMigrationState(marker.Config.Metadata)
		if perr != nil {
			return perr
		}
		if st.target != target {
			return fmt.Errorf("%w: bucket %q: resumed with a configuration other than the interrupted migration's",
				ErrBucketMigrationConflict, cfg.Bucket)
		}
		st.lease = lease
		return h.runMigration(ctx, st)
	}

	st, err := h.prepareMigration(ctx, cfg, target, lease)
	if err != nil || st == nil {
		return err
	}
	return h.runMigration(ctx, st)
}

// checkMigrationContext rejects JetStream contexts whose subjects the raw
// publishes of a migration would not route like the KeyValue API does.
func (h *KVHelper) checkMigrationContext() error {
	opts := h.js.Options()
	if opts.Domain != "" || (opts.APIPrefix != "" && opts.APIPrefix != jetstream.DefaultAPIPrefix) {
		return ErrMigrationUnsupportedContext
	}
	return nil
}

// prepareMigration validates a fresh migration and creates its marker. It
// returns a nil state when there is nothing to migrate.
func (h *KVHelper) prepareMigration(ctx context.Context, cfg BucketSpec, target migrationTarget, lease *migrationLease) (*migrationState, error) {
	if err := lease.renew(ctx); err != nil {
		return nil, err
	}
	if err := h.cleanupTemplates(ctx, cfg.Bucket); err != nil {
		return nil, err
	}

	src, err := h.streamInfo(ctx, kvStreamName(cfg.Bucket))
	if err != nil || src == nil {
		return nil, err
	}
	scfg := src.Config

	if scfg.Sealed {
		return nil, fmt.Errorf("%w: bucket %q is sealed without a migration marker; its original TTL is unknown",
			ErrBucketMigrationConflict, cfg.Bucket)
	}
	if len(scfg.Sources) > 0 || scfg.RePublish != nil || scfg.SubjectTransform != nil || scfg.Placement != nil {
		return nil, fmt.Errorf("%w: bucket %q uses sources, republishing, a subject transform or a placement",
			ErrMigrationUnsupportedBucket, cfg.Bucket)
	}
	kind := kindStandalone
	if scfg.Mirror != nil {
		kind = kindMirror
	}
	if scfg.MaxAge != cfg.TTL && !cfg.MigrateTTL {
		return nil, fmt.Errorf("%w: bucket %q has key TTL %s, configured %s",
			ErrBucketTTLMismatch, cfg.Bucket, scfg.MaxAge, cfg.TTL)
	}
	if scfg.Storage == cfg.Storage {
		return nil, nil //nolint:nilnil // nothing to migrate
	}

	id := newMigrationID()
	templateBucket := migrationTemplatePrefix + id
	if err = lease.renew(ctx); err != nil {
		return nil, err
	}
	targetStream, err := h.templateStreamConfig(ctx, cfg, &scfg, id, templateBucket)
	if err != nil {
		return nil, err
	}

	st := &migrationState{
		id:            id,
		phase:         phasePrepared,
		sourceTTL:     scfg.MaxAge,
		sourceStorage: scfg.Storage,
		sourceCreated: src.Created.UnixNano(),
		target:        target,
		targetStream:  targetStream,
		kind:          kind,
		lease:         lease,
	}
	meta, err := st.metadata(cfg.Bucket, templateBucket)
	if err != nil {
		return nil, err
	}
	if err = lease.renew(ctx); err != nil {
		return nil, err
	}

	_, err = h.js.CreateStream(ctx, jetstream.StreamConfig{
		Name:              markerStreamName(cfg.Bucket),
		Subjects:          []string{markerSubjectPrefix(cfg.Bucket) + ">"},
		Storage:           jetstream.FileStorage,
		Replicas:          max(scfg.Replicas, 1),
		MaxMsgsPerSubject: 1,
		Metadata:          meta,
	})
	if errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		return nil, fmt.Errorf("%w: bucket %q", ErrBucketMigrationInProgress, cfg.Bucket)
	}
	if err != nil {
		return nil, fmt.Errorf("create migration marker: %w", err)
	}

	h.logger.InfoContext(ctx, "KeyValue bucket storage migration started",
		slog.String("bucket", cfg.Bucket), slog.String("migration_id", id),
		slog.String("from", scfg.Storage.String()), slog.String("to", cfg.Storage.String()))

	return st, nil
}

// templateStreamConfig lets the client build and the server validate the
// target bucket's stream configuration by creating an owned template bucket,
// then deletes the template. It runs before the source is touched, so an
// invalid target fails without consequences.
func (h *KVHelper) templateStreamConfig(
	ctx context.Context, cfg BucketSpec, source *jetstream.StreamConfig, id, templateBucket string,
) (jetstream.StreamConfig, error) {
	meta := map[string]string{metaID: id, metaRole: roleTemplate, metaBucket: cfg.Bucket}

	var err error
	if source.Mirror != nil {
		_, err = h.js.CreateStream(ctx, mirrorTemplateConfig(cfg, source, templateBucket, meta))
	} else {
		kvCfg := h.keyValueConfig(cfg)
		kvCfg.Bucket = templateBucket
		kvCfg.Metadata = meta
		_, err = h.js.CreateKeyValue(ctx, kvCfg)
	}
	if err != nil {
		return jetstream.StreamConfig{}, fmt.Errorf("validate target bucket configuration: %w", err)
	}
	info, err := h.streamInfo(ctx, kvStreamName(templateBucket))
	if err == nil && info == nil {
		err = jetstream.ErrStreamNotFound
	}
	if err != nil {
		return jetstream.StreamConfig{}, err
	}
	if err := h.deleteStream(ctx, kvStreamName(templateBucket)); err != nil {
		return jetstream.StreamConfig{}, err
	}
	return info.Config, nil
}

// mirrorTemplateConfig derives a mirror's target stream from the mirror
// itself, so everything that makes it that mirror — the Mirror source,
// MirrorDirect, history — is kept, and only what the migration targets
// changes. Delete markers are left off: the server forbids them on mirrors,
// whose entries expire as the origin's do.
func mirrorTemplateConfig(cfg BucketSpec, source *jetstream.StreamConfig, templateBucket string, meta map[string]string) jetstream.StreamConfig {
	c := *source
	c.Name = kvStreamName(templateBucket)
	c.Storage = cfg.Storage
	c.MaxAge = cfg.TTL
	c.Replicas = max(cfg.Replicas, 1)
	c.Compression = jetstream.NoCompression
	if cfg.Compression {
		c.Compression = jetstream.S2Compression
	}
	c.Sealed = false
	c.FirstSeq = 0
	c.SubjectDeleteMarkerTTL = 0
	c.Metadata = meta
	return c
}

// cleanupTemplates deletes template buckets left by fresh migration runs of
// bucket that crashed before creating their marker. Only streams that prove
// to be such templates are deleted.
func (h *KVHelper) cleanupTemplates(ctx context.Context, bucket string) error {
	lister := h.js.ListStreams(ctx)
	var stale []string
	for info := range lister.Info() {
		meta := info.Config.Metadata
		name := info.Config.Name
		if meta[metaRole] == roleTemplate && meta[metaBucket] == bucket && meta[metaID] != "" &&
			name == kvStreamName(migrationTemplatePrefix+meta[metaID]) && name != kvStreamName(bucket) {
			stale = append(stale, name)
		}
	}
	if err := lister.Err(); err != nil && !errors.Is(err, jetstream.ErrEndOfData) {
		return fmt.Errorf("list streams: %w", err)
	}
	for _, name := range stale {
		if err := h.deleteStream(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// runMigration drives the migration from its recorded phase to the end.
func (h *KVHelper) runMigration(ctx context.Context, st *migrationState) error {
	for {
		// Self-fence: stop before mutating anything once the lease is lost.
		if err := st.lease.renew(ctx); err != nil {
			return err
		}
		var err error
		switch st.phase {
		case phasePrepared:
			if st.kind == kindMirror {
				// A mirror takes no direct writes and is re-synced from its
				// origin: nothing to seal or copy.
				err = h.advance(ctx, st, phaseCopied)
			} else {
				err = h.sealSource(ctx, st)
			}
		case phaseSealed:
			err = h.copyEntries(ctx, st)
		case phaseCopied:
			err = h.deleteSource(ctx, st)
		case phaseSourceDeleted:
			err = h.createReplacement(ctx, st)
		case phaseReplacementCreated:
			if st.kind == kindMirror {
				err = h.advance(ctx, st, phaseRestored)
			} else {
				err = h.restoreEntries(ctx, st)
			}
		case phaseRestored:
			return h.finishMigration(ctx, st)
		default:
			return fmt.Errorf("%w: bucket %q: unknown migration phase %q", ErrBucketMigrationConflict, st.target.Bucket, st.phase)
		}
		if err != nil {
			return err
		}
		if err := h.fail("after:" + st.phase); err != nil {
			return err
		}
	}
}

// finishMigration deletes the marker once the replacement is verifiably in
// place. The marker is the last record of the revision floor, so it is kept
// otherwise.
func (h *KVHelper) finishMigration(ctx context.Context, st *migrationState) error {
	info, err := h.streamInfo(ctx, kvStreamName(st.target.Bucket))
	if err != nil {
		return err
	}
	if err := h.checkReplacement(st, info); err != nil {
		return err
	}
	if err := h.deleteStream(ctx, markerStreamName(st.target.Bucket)); err != nil {
		return err
	}
	h.logger.InfoContext(ctx, "KeyValue bucket storage migration finished",
		slog.String("bucket", st.target.Bucket), slog.String("migration_id", st.id))
	return nil
}

// source returns the stream that is this migration's source, nil when the
// bucket stream is gone, or ErrBucketMigrationConflict for any other stream.
func (h *KVHelper) source(ctx context.Context, st *migrationState) (*jetstream.StreamInfo, error) {
	info, err := h.streamInfo(ctx, kvStreamName(st.target.Bucket))
	if err != nil || info == nil {
		return nil, err
	}
	if info.Config.Metadata[metaID] == st.id || info.Created.UnixNano() != st.sourceCreated {
		return nil, fmt.Errorf("%w: bucket %q is not the source of this migration", ErrBucketMigrationConflict, st.target.Bucket)
	}
	return info, nil
}

// advance records the next phase in the marker.
func (h *KVHelper) advance(ctx context.Context, st *migrationState, phase string) error {
	marker, err := h.streamInfo(ctx, markerStreamName(st.target.Bucket))
	if err != nil {
		return err
	}
	if marker == nil || marker.Config.Metadata[metaID] != st.id {
		return fmt.Errorf("%w: bucket %q: migration marker is gone or replaced", ErrBucketMigrationConflict, st.target.Bucket)
	}

	next := *st
	next.phase = phase
	meta, err := next.metadata(st.target.Bucket, marker.Config.Metadata[metaTemplateBucket])
	if err != nil {
		return err
	}
	cfg := marker.Config
	cfg.Metadata = maps.Clone(cfg.Metadata)
	maps.Copy(cfg.Metadata, meta)
	if _, err := h.js.UpdateStream(ctx, cfg); err != nil {
		return fmt.Errorf("update migration marker: %w", err)
	}
	st.phase = phase
	return nil
}

func (st *migrationState) metadata(bucket, templateBucket string) (map[string]string, error) {
	target, err := json.Marshal(st.target)
	if err != nil {
		return nil, err
	}
	stream, err := json.Marshal(st.targetStream)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		metaID:             st.id,
		metaRole:           roleMarker,
		metaBucket:         bucket,
		metaPhase:          st.phase,
		metaSourceTTL:      strconv.FormatInt(int64(st.sourceTTL), 10),
		metaSourceStorage:  strconv.Itoa(int(st.sourceStorage)),
		metaSourceCreated:  strconv.FormatInt(st.sourceCreated, 10),
		metaSourceLastSeq:  strconv.FormatUint(st.sourceLastSeq, 10),
		metaTarget:         string(target),
		metaTargetStream:   string(stream),
		metaTemplateBucket: templateBucket,
		metaCopied:         strconv.Itoa(st.copied),
		metaKind:           st.kind,
	}, nil
}

func parseMigrationState(meta map[string]string) (*migrationState, error) {
	bad := func(key string, err error) error {
		return fmt.Errorf("%w: migration marker has a bad %s: %v", ErrBucketMigrationConflict, key, err)
	}
	st := &migrationState{id: meta[metaID], phase: meta[metaPhase], kind: meta[metaKind]}
	if st.kind == "" {
		st.kind = kindStandalone // markers written before mirrors were supported
	}
	if st.kind != kindStandalone && st.kind != kindMirror {
		return nil, fmt.Errorf("%w: migration marker has an unknown kind %q", ErrBucketMigrationConflict, st.kind)
	}
	if st.id == "" || meta[metaRole] != roleMarker {
		return nil, fmt.Errorf("%w: stream is not a migration marker", ErrBucketMigrationConflict)
	}
	ttl, err := strconv.ParseInt(meta[metaSourceTTL], 10, 64)
	if err != nil {
		return nil, bad(metaSourceTTL, err)
	}
	st.sourceTTL = time.Duration(ttl)
	storage, err := strconv.Atoi(meta[metaSourceStorage])
	if err != nil {
		return nil, bad(metaSourceStorage, err)
	}
	st.sourceStorage = jetstream.StorageType(storage)
	if st.sourceCreated, err = strconv.ParseInt(meta[metaSourceCreated], 10, 64); err != nil {
		return nil, bad(metaSourceCreated, err)
	}
	if st.sourceLastSeq, err = strconv.ParseUint(meta[metaSourceLastSeq], 10, 64); err != nil {
		return nil, bad(metaSourceLastSeq, err)
	}
	if st.copied, err = strconv.Atoi(meta[metaCopied]); err != nil {
		return nil, bad(metaCopied, err)
	}
	if err := json.Unmarshal([]byte(meta[metaTarget]), &st.target); err != nil {
		return nil, bad(metaTarget, err)
	}
	if err := json.Unmarshal([]byte(meta[metaTargetStream]), &st.targetStream); err != nil {
		return nil, bad(metaTargetStream, err)
	}
	return st, nil
}

// streamInfo returns the stream's info, or nil when it does not exist.
func (h *KVHelper) streamInfo(ctx context.Context, name string) (*jetstream.StreamInfo, error) {
	stream, err := h.js.Stream(ctx, name)
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		return nil, nil //nolint:nilnil // a missing stream is not an error here
	}
	if err != nil {
		return nil, fmt.Errorf("look up stream %q: %w", name, err)
	}
	return stream.Info(ctx)
}

func (h *KVHelper) deleteStream(ctx context.Context, name string) error {
	if err := h.js.DeleteStream(ctx, name); err != nil && !errors.Is(err, jetstream.ErrStreamNotFound) {
		return fmt.Errorf("delete stream %q: %w", name, err)
	}
	return nil
}

func (h *KVHelper) fail(step string) error {
	if h.failpoint == nil {
		return nil
	}
	return h.failpoint(step)
}

func (h *KVHelper) clock() time.Time {
	if h.now == nil {
		return time.Now()
	}
	return h.now()
}

// migrationIDBytes is the number of random bytes in a migration or lease
// owner ID; the hex form is twice as long.
const migrationIDBytes = 8

// newMigrationID returns a random migration or lease owner ID: migrationIDBytes
// of crypto/rand entropy as lowercase hex, safe in a bucket name.
// crypto/rand.Read never fails since Go 1.24.
func newMigrationID() string {
	b := make([]byte, migrationIDBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func kvStreamName(bucket string) string        { return "KV_" + bucket }
func kvSubjectPrefix(bucket string) string     { return "$KV." + bucket + "." }
func markerStreamName(bucket string) string    { return migrationMarkerPrefix + bucket }
func markerSubjectPrefix(bucket string) string { return migrationSubjectPrefix + bucket + "." }
