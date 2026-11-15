// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

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
		if st.kind != kindMirror {
			cfg.Subjects = []string{kvSubjectPrefix(st.target.Bucket) + ">"}
			cfg.FirstSeq = st.sourceLastSeq + 1
		}
		// A mirror keeps no subjects of its own and the origin's sequence
		// numbers, so its revisions stay those of the origin.
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
	if st.kind == kindMirror {
		return nil // revisions are the origin's
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
		if seq > mi.State.FirstSeq && (seq-mi.State.FirstSeq)%leaseRenewEveryEntries == 0 {
			if err := st.lease.renew(ctx); err != nil {
				return err
			}
		}
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

	msgTTL, err := parseCopiedTTL(msg.Header.Get(headerMsgTTL))
	if err != nil {
		return fmt.Errorf("migration marker entry %q: bad %s header: %w", key, headerMsgTTL, err)
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
