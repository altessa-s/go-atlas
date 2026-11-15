// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

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
	for i, key := range keys {
		if i > 0 && i%leaseRenewEveryEntries == 0 {
			if err := st.lease.renew(ctx); err != nil {
				return copied, err
			}
		}
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
		ttlHeader, err := copiedTTLHeader(msg.Header.Get(msgTTLHeader))
		if err != nil {
			return copied, fmt.Errorf("key %q: %w", key, err)
		}
		if ttlHeader != "" {
			out.Header.Set(headerMsgTTL, ttlHeader)
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
		return d.Truncate(time.Second), nil
	}
	return parseTTLSeconds(raw)
}

// parseTTLSeconds is shared by source and marker headers so both reject
// negative or overflowing TTLs before converting seconds to nanoseconds.
func parseTTLSeconds(raw string) (time.Duration, error) {
	secs, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || secs < 0 || secs > math.MaxInt64/int64(time.Second) {
		return 0, fmt.Errorf("invalid per-message TTL %q", raw)
	}
	return time.Duration(secs) * time.Second, nil
}

// copiedTTLHeader encodes a source entry's Nats-TTL header as the marker's
// Kv-Migrate-Msg-TTL header: "" for no TTL of its own, "never", or whole
// seconds.
func copiedTTLHeader(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	ttl, err := parseMessageTTL(raw)
	if err != nil {
		return "", err
	}
	switch {
	case ttl < 0:
		return msgTTLNever, nil
	case ttl > 0:
		return strconv.FormatInt(int64(ttl/time.Second), 10), nil
	}
	return "", nil
}

// parseCopiedTTL decodes a Kv-Migrate-Msg-TTL header written by
// copiedTTLHeader: 0 for none, -1 for never, the TTL otherwise.
func parseCopiedTTL(raw string) (time.Duration, error) {
	switch raw {
	case "":
		return 0, nil
	case msgTTLNever:
		return -1, nil
	}
	return parseTTLSeconds(raw)
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
	case info.Created.UnixNano() != st.sourceCreated || (st.kind != kindMirror && !info.Config.Sealed):
		return fmt.Errorf("%w: bucket %q is not the sealed source of this migration", ErrBucketMigrationConflict, st.target.Bucket)
	default:
		if err := h.deleteStream(ctx, kvStreamName(st.target.Bucket)); err != nil {
			return err
		}
	}
	return h.advance(ctx, st, phaseSourceDeleted)
}
