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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
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

// ErrMigrationUnsupportedBucket reports a bucket whose stream uses a mirror,
// sources, republishing, a subject transform or a placement. The migrated
// bucket would lose them, so such buckets are not migrated.
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
func (h *KVHelper) MigrateBucketStorage(ctx context.Context, cfg BucketConfig, copyEntries bool, opts MigrationOptions) error {
	if err := h.checkMigrationContext(); err != nil {
		return err
	}

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
		return h.runMigration(ctx, st)
	}

	st, err := h.prepareMigration(ctx, cfg, target)
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
func (h *KVHelper) prepareMigration(ctx context.Context, cfg BucketConfig, target migrationTarget) (*migrationState, error) {
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
	if scfg.Mirror != nil || len(scfg.Sources) > 0 || scfg.RePublish != nil || scfg.SubjectTransform != nil || scfg.Placement != nil {
		return nil, fmt.Errorf("%w: bucket %q", ErrMigrationUnsupportedBucket, cfg.Bucket)
	}
	if scfg.MaxAge != cfg.TTL && !cfg.MigrateTTL {
		return nil, fmt.Errorf("%w: bucket %q has key TTL %s, configured %s",
			ErrBucketTTLMismatch, cfg.Bucket, scfg.MaxAge, cfg.TTL)
	}
	if scfg.Storage == cfg.Storage {
		return nil, nil //nolint:nilnil // nothing to migrate
	}

	id, err := newMigrationID()
	if err != nil {
		return nil, err
	}
	templateBucket := migrationTemplatePrefix + id
	targetStream, err := h.templateStreamConfig(ctx, cfg, id, templateBucket)
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
	}
	meta, err := st.metadata(cfg.Bucket, templateBucket)
	if err != nil {
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
func (h *KVHelper) templateStreamConfig(ctx context.Context, cfg BucketConfig, id, templateBucket string) (jetstream.StreamConfig, error) {
	kvCfg := h.keyValueConfig(cfg)
	kvCfg.Bucket = templateBucket
	kvCfg.Metadata = map[string]string{metaID: id, metaRole: roleTemplate, metaBucket: cfg.Bucket}

	if _, err := h.js.CreateKeyValue(ctx, kvCfg); err != nil {
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
		var err error
		switch st.phase {
		case phasePrepared:
			err = h.sealSource(ctx, st)
		case phaseSealed:
			err = h.copyEntries(ctx, st)
		case phaseCopied:
			err = h.deleteSource(ctx, st)
		case phaseSourceDeleted:
			err = h.createReplacement(ctx, st)
		case phaseReplacementCreated:
			err = h.restoreEntries(ctx, st)
		case phaseRestored:
			// The marker is the last record of the revision floor: keep it
			// unless the replacement is verifiably in place.
			info, ierr := h.streamInfo(ctx, kvStreamName(st.target.Bucket))
			if ierr != nil {
				return ierr
			}
			if cerr := h.checkReplacement(st, info); cerr != nil {
				return cerr
			}
			if derr := h.deleteStream(ctx, markerStreamName(st.target.Bucket)); derr != nil {
				return derr
			}
			h.logger.InfoContext(ctx, "KeyValue bucket storage migration finished",
				slog.String("bucket", st.target.Bucket), slog.String("migration_id", st.id))
			return nil
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

// sealSource makes the source read-only and records its last revision.
func (h *KVHelper) sealSource(ctx context.Context, st *migrationState) error {
	if err := h.fail("before-seal"); err != nil {
		return err
	}
	src, err := h.source(ctx, st)
	if err != nil {
		return err
	}
	if src == nil {
		return fmt.Errorf("%w: bucket %q disappeared before it was sealed", ErrBucketMigrationConflict, st.target.Bucket)
	}
	if !src.Config.Sealed {
		// Sealing freezes the source. Turning direct gets off with it makes
		// every read of the copy leader-served: a direct get may be answered
		// by a replica that has not caught up and return a stale value.
		cfg := src.Config
		cfg.Sealed = true
		cfg.AllowDirect = false
		updated, err := h.js.UpdateStream(ctx, cfg)
		if err != nil {
			return fmt.Errorf("seal bucket: %w", err)
		}
		src = updated.CachedInfo()
	}
	if err := h.fail("sealed"); err != nil {
		return err
	}
	st.sourceLastSeq = src.State.LastSeq
	return h.advance(ctx, st, phaseSealed)
}

// copyEntries copies the live entries of the sealed source into the marker.
func (h *KVHelper) copyEntries(ctx context.Context, st *migrationState) error {
	if st.target.CopyEntries {
		src, err := h.source(ctx, st)
		if err != nil {
			return err
		}
		if src == nil || !src.Config.Sealed || src.Config.AllowDirect {
			return fmt.Errorf("%w: bucket %q is missing, not sealed or allows direct gets while copying",
				ErrBucketMigrationConflict, st.target.Bucket)
		}
		n, err := h.copySource(ctx, st)
		if err != nil {
			return err
		}
		st.copied = n
	}
	return h.advance(ctx, st, phaseCopied)
}

func (h *KVHelper) copySource(ctx context.Context, st *migrationState) (int, error) {
	bucket := st.target.Bucket
	stream, err := h.js.Stream(ctx, kvStreamName(bucket))
	if err != nil {
		return 0, err
	}
	// The inventory is the server's own per-subject state of the sealed
	// stream, fetched page by page until complete; a watcher-based key list
	// could end early without reporting it.
	info, err := stream.Info(ctx, jetstream.WithSubjectFilter(kvSubjectPrefix(bucket)+">"))
	if err != nil {
		return 0, fmt.Errorf("list keys: %w", err)
	}
	if uint64(len(info.State.Subjects)) != info.State.NumSubjects {
		return 0, fmt.Errorf("list keys: inventory has %d of %d subjects", len(info.State.Subjects), info.State.NumSubjects)
	}
	if err := h.fail("inventory"); err != nil {
		return 0, err
	}
	keys := make([]string, 0, len(info.State.Subjects))
	for subject := range info.State.Subjects {
		keys = append(keys, strings.TrimPrefix(subject, kvSubjectPrefix(bucket)))
	}
	slices.Sort(keys)

	copied := 0
	for _, key := range keys {
		// Leader-served (direct gets are off on the sealed source). A key in
		// the inventory that cannot be read is an error, not a skip.
		msg, err := stream.GetLastMsgForSubject(ctx, kvSubjectPrefix(bucket)+key)
		if err != nil {
			return copied, fmt.Errorf("read key %q: %w", key, err)
		}
		if msg.Header.Get(kvOperationHeader) != "" || msg.Header.Get(markerReasonHeader) != "" {
			continue
		}

		out := nats.NewMsg(markerSubjectPrefix(bucket) + key)
		out.Data = msg.Data
		out.Header.Set(headerTime, strconv.FormatInt(msg.Time.UnixNano(), 10))
		if raw := msg.Header.Get(msgTTLHeader); raw != "" {
			ttl, err := parseMessageTTL(raw)
			if err != nil {
				return copied, fmt.Errorf("key %q: %w", key, err)
			}
			switch {
			case ttl < 0:
				out.Header.Set(headerMsgTTL, msgTTLNever)
			case ttl > 0:
				out.Header.Set(headerMsgTTL, strconv.FormatInt(int64(ttl/time.Second), 10))
			}
		}
		if _, err := h.js.PublishMsg(ctx, out); err != nil {
			return copied, fmt.Errorf("copy key %q: %w", key, err)
		}
		copied++
		if err := h.fail("copied:" + key); err != nil {
			return copied, err
		}
	}
	return copied, nil
}

// parseMessageTTL interprets a Nats-TTL header exactly as nats-server does:
// "never" is -1, a Go duration is truncated to whole seconds (sub-second
// values are invalid), anything else is an integer number of seconds. Zero
// means the message has no TTL of its own.
func parseMessageTTL(raw string) (time.Duration, error) {
	if strings.EqualFold(raw, msgTTLNever) {
		return -1, nil
	}
	if d, err := time.ParseDuration(raw); err == nil {
		if d < time.Second {
			return 0, fmt.Errorf("invalid per-message TTL %q", raw)
		}
		return time.Duration(int64(d.Seconds())) * time.Second, nil
	}
	secs, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || secs < 0 {
		return 0, fmt.Errorf("invalid per-message TTL %q", raw)
	}
	return time.Duration(secs) * time.Second, nil
}

// deleteSource deletes the sealed source once its entries are in the marker.
func (h *KVHelper) deleteSource(ctx context.Context, st *migrationState) error {
	info, err := h.streamInfo(ctx, kvStreamName(st.target.Bucket))
	if err != nil {
		return err
	}
	switch {
	case info == nil:
	case info.Config.Metadata[metaID] == st.id:
		// Already replaced by this migration.
	case info.Created.UnixNano() != st.sourceCreated || !info.Config.Sealed:
		return fmt.Errorf("%w: bucket %q is not the sealed source of this migration", ErrBucketMigrationConflict, st.target.Bucket)
	default:
		if err := h.deleteStream(ctx, kvStreamName(st.target.Bucket)); err != nil {
			return err
		}
	}
	return h.advance(ctx, st, phaseSourceDeleted)
}

// createReplacement creates the target bucket with its first revision just
// above the source's last one, atomically, so no write to it can ever get a
// revision the source already handed out.
func (h *KVHelper) createReplacement(ctx context.Context, st *migrationState) error {
	name := kvStreamName(st.target.Bucket)
	info, err := h.streamInfo(ctx, name)
	if err != nil {
		return err
	}
	if info == nil {
		cfg := st.targetStream
		cfg.Name = name
		cfg.Subjects = []string{kvSubjectPrefix(st.target.Bucket) + ">"}
		cfg.FirstSeq = st.sourceLastSeq + 1
		meta := make(map[string]string, len(cfg.Metadata)+2)
		for k, v := range cfg.Metadata {
			if !strings.HasPrefix(k, metaPrefix) {
				meta[k] = v
			}
		}
		meta[metaID] = st.id
		meta[metaRole] = roleReplacement
		cfg.Metadata = meta

		_, err := h.js.CreateStream(ctx, cfg)
		if err != nil && !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
			return fmt.Errorf("create migrated bucket: %w", err)
		}
		if info, err = h.streamInfo(ctx, name); err != nil {
			return err
		}
	}
	if err := h.checkReplacement(st, info); err != nil {
		return err
	}
	return h.advance(ctx, st, phaseReplacementCreated)
}

func (h *KVHelper) checkReplacement(st *migrationState, info *jetstream.StreamInfo) error {
	if info == nil || info.Config.Metadata[metaID] != st.id {
		return fmt.Errorf("%w: bucket %q is not the replacement of this migration", ErrBucketMigrationConflict, st.target.Bucket)
	}
	if info.Config.Storage != st.target.Storage || info.Config.MaxAge != st.target.TTL {
		return fmt.Errorf("%w: bucket %q replacement does not match the migration target", ErrBucketMigrationConflict, st.target.Bucket)
	}
	// The revision floor: every revision the replacement holds or hands out
	// must be above the source's last one.
	floor := st.sourceLastSeq
	if info.State.LastSeq < floor || (info.State.Msgs > 0 && info.State.FirstSeq <= floor) {
		return fmt.Errorf("%w: bucket %q replacement has revisions at or below the source's last revision %d",
			ErrBucketMigrationConflict, st.target.Bucket, floor)
	}
	return nil
}

// restoreEntries writes the copied entries into the replacement.
func (h *KVHelper) restoreEntries(ctx context.Context, st *migrationState) error {
	info, err := h.streamInfo(ctx, kvStreamName(st.target.Bucket))
	if err != nil {
		return err
	}
	if err = h.checkReplacement(st, info); err != nil {
		return err
	}

	marker, err := h.js.Stream(ctx, markerStreamName(st.target.Bucket))
	if err != nil {
		return fmt.Errorf("open migration marker: %w", err)
	}
	mi, err := marker.Info(ctx)
	if err != nil {
		return err
	}
	prefix := markerSubjectPrefix(st.target.Bucket)
	for seq := mi.State.FirstSeq; seq <= mi.State.LastSeq && mi.State.Msgs > 0; seq++ {
		msg, err := marker.GetMsg(ctx, seq)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read migration marker: %w", err)
		}
		key, ok := strings.CutPrefix(msg.Subject, prefix)
		if !ok {
			continue
		}
		if err := h.restoreEntry(ctx, st, key, msg); err != nil {
			return err
		}
	}
	return h.advance(ctx, st, phaseRestored)
}

// restoreEntry publishes one entry into the replacement, only if its key has
// no message yet, with the lifetime it has left. The remaining lifetime is
// recomputed before every attempt, and an entry past its deadline is skipped.
// Deadlines are server timestamps compared with the migrator's clock, so they
// are as accurate as the two clocks are synchronized.
func (h *KVHelper) restoreEntry(ctx context.Context, st *migrationState, key string, msg *jetstream.RawStreamMsg) error {
	ts, err := strconv.ParseInt(msg.Header.Get(headerTime), 10, 64)
	if err != nil {
		return fmt.Errorf("migration marker entry %q: bad %s header: %w", key, headerTime, err)
	}
	written := time.Unix(0, ts)

	var msgTTL time.Duration // 0: none, -1: never
	switch raw := msg.Header.Get(headerMsgTTL); raw {
	case "":
	case msgTTLNever:
		msgTTL = -1
	default:
		secs, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("migration marker entry %q: bad %s header: %w", key, headerMsgTTL, err)
		}
		msgTTL = time.Duration(secs) * time.Second
	}
	perKey := msgTTL != 0 && st.target.LimitMarkerTTL > 0

	var lastErr error
	for attempt := range restoreAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(restoreBackoff << (attempt - 1)):
			}
		}

		ttlHeader, live := restoreLifetime(st, written, msgTTL, perKey, h.clock())
		if !live {
			return nil
		}

		out := nats.NewMsg(kvSubjectPrefix(st.target.Bucket) + key)
		out.Data = msg.Data
		if ttlHeader != "" {
			out.Header.Set(msgTTLHeader, ttlHeader)
		}

		if lastErr = h.fail("restore:" + key); lastErr == nil {
			_, lastErr = h.js.PublishMsg(ctx, out,
				jetstream.WithExpectLastSequencePerSubject(0),
				jetstream.WithRetryAttempts(0))
		}
		var apiErr *jetstream.APIError
		switch {
		case lastErr == nil:
			return nil
		case errors.As(lastErr, &apiErr) && apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence:
			// The key already has a message: restored by an earlier run, or
			// written or deleted by someone else. Never overwrite it.
			return nil
		case errors.Is(lastErr, nats.ErrNoResponders), errors.Is(lastErr, nats.ErrTimeout), errors.Is(lastErr, context.DeadlineExceeded):
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		default:
			return fmt.Errorf("restore key %q: %w", key, lastErr)
		}
	}
	return fmt.Errorf("restore key %q: %w", key, lastErr)
}

// restoreLifetime decides whether an entry written at written is still alive
// at now, and the Nats-TTL header it is restored with ("" for none).
func restoreLifetime(st *migrationState, written time.Time, msgTTL time.Duration, perKey bool, now time.Time) (string, bool) {
	if msgTTL < 0 {
		if perKey {
			return msgTTLNever, true
		}
		return "", true // the target has no per-message TTL: a plain entry
	}

	if perKey {
		deadline := written.Add(msgTTL)
		if st.target.TTL == st.sourceTTL && st.sourceTTL > 0 {
			deadline = minTime(deadline, written.Add(st.sourceTTL))
		}
		remaining := deadline.Sub(now)
		if remaining <= 0 {
			return "", false
		}
		secs := (remaining + time.Second - 1) / time.Second
		return strconv.FormatInt(int64(secs), 10) + "s", true
	}

	// A plain entry lives the bucket TTL from the restore; skip it only if it
	// would already have expired under the target TTL.
	if st.target.TTL > 0 && !written.Add(st.target.TTL).After(now) {
		return "", false
	}
	return "", true
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
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
	}, nil
}

func parseMigrationState(meta map[string]string) (*migrationState, error) {
	bad := func(key string, err error) error {
		return fmt.Errorf("%w: migration marker has a bad %s: %v", ErrBucketMigrationConflict, key, err)
	}
	st := &migrationState{id: meta[metaID], phase: meta[metaPhase]}
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

func newMigrationID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func kvStreamName(bucket string) string        { return "KV_" + bucket }
func kvSubjectPrefix(bucket string) string     { return "$KV." + bucket + "." }
func markerStreamName(bucket string) string    { return migrationMarkerPrefix + bucket }
func markerSubjectPrefix(bucket string) string { return migrationSubjectPrefix + bucket + "." }
