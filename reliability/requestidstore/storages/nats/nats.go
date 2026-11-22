// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-aip/requestid"

	"github.com/altessa-s/go-atlas/internal/nats/natsbase"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/internal/purgeloop"
	"github.com/altessa-s/go-atlas/reliability/requestidstore/internal/watermark"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrBucketTTLMismatch indicates a bucket whose TTL could expire a live entry.
var ErrBucketTTLMismatch = natskvlease.ErrBucketTTLMismatch

// ErrBucketStorageMismatch indicates an existing bucket with a different storage type.
var ErrBucketStorageMismatch = natskvlease.ErrBucketStorageMismatch

// Storage keeps one entry per request ID in a dedicated JetStream KV bucket.
type Storage struct {
	natsbase.Base
	cleanupGrace time.Duration
	stream       jetstream.Stream
	clock        watermark.Clock
	purge        *purgeloop.Loop
}

var _ requestid.Store = (*Storage)(nil)

// New opens or creates a file-backed bucket and refuses an existing bucket with a key TTL.
func New(ctx context.Context, js jetstream.JetStream, opts ...Option) (*Storage, error) {
	if js == nil {
		return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "JetStream is required")
	}
	o := newOptions(opts...)
	base, err := natsbase.NewBaseWithBucket(ctx, js, natskvlease.BucketSpec{
		Bucket: o.bucket, NoTTL: true, Storage: jetstream.FileStorage, Replicas: o.replicas,
	}, o.logger)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "open request ID bucket")
	}
	stream, err := js.Stream(ctx, "KV_"+o.bucket)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "open request ID stream")
	}
	cfg := stream.CachedInfo().Config
	if cfg.Discard == jetstream.DiscardOld && (cfg.MaxBytes > 0 || cfg.MaxMsgs > 0) {
		return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "request ID bucket may evict live entries")
	}
	s := &Storage{Base: base, stream: stream, cleanupGrace: o.cleanupGrace}
	s.purge = purgeloop.New(o.purgeInterval, s.purgeExpired, o.logger, "request IDs")
	return s, nil
}

// Close stops background cleanup. The caller retains the JetStream connection.
func (s *Storage) Close(ctx context.Context) error { return s.purge.Close(ctx) }

func (s *Storage) observe(ctx context.Context, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.clock.Observe(now)
	s.purge.Start() //nolint:contextcheck // Purging outlives the caller's request.
	return nil
}

// latest reads a leader-backed consumer snapshot. Direct KV.Get on a replicated
// bucket can return a stale follower value, including after a successful Release.
func (s *Storage) latest(ctx context.Context, key string) (requestid.Entry, uint64, error) {
	w, err := s.KV().Watch(ctx, key)
	if err != nil {
		return requestid.Entry{}, 0, coreerrs.WrapOperation(err, "watch request ID")
	}
	defer func() { _ = w.Stop() }()
	item, err := next(ctx, w)
	if err != nil || item == nil {
		return requestid.Entry{}, 0, err
	}
	if item.Operation() != jetstream.KeyValuePut {
		return requestid.Entry{}, item.Revision(), nil
	}
	e, err := decode(item.Value())
	return e, item.Revision(), err
}

func next(ctx context.Context, w jetstream.KeyWatcher) (jetstream.KeyValueEntry, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case item, ok := <-w.Updates():
		if !ok {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return nil, io.ErrUnexpectedEOF
		}
		return item, nil
	}
}

// Reserve installs pending if the current revision is absent or expired at now.
func (s *Storage) Reserve(ctx context.Context, key string, pending requestid.Entry, now time.Time) (requestid.Entry, bool, error) {
	if err := s.observe(ctx, now); err != nil {
		return requestid.Entry{}, false, err
	}
	data, err := encode(pending)
	if err != nil {
		return requestid.Entry{}, false, err
	}
	for {
		current, rev, err := s.latest(ctx, key)
		if err != nil {
			return requestid.Entry{}, false, err
		}
		if current.State != 0 && now.Before(current.ExpiresAt) {
			return current, false, nil
		}
		// Revision zero expects a missing subject. A release marker carries
		// its own revision and must be replaced conditionally too.
		_, err = s.KV().Update(ctx, key, data, rev)
		if err == nil {
			return pending, true, nil
		}
		if !errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
			return requestid.Entry{}, false, coreerrs.WrapOperation(err, "reserve request ID")
		}
	}
}

// Complete replaces the caller's live pending revision with done.
func (s *Storage) Complete(ctx context.Context, key, token string, done requestid.Entry, now time.Time) error {
	return s.transition(ctx, key, token, now, func(rev uint64) error {
		data, err := encode(done)
		if err != nil {
			return err
		}
		_, err = s.KV().Update(ctx, key, data, rev)
		return err
	})
}

// Release purges the caller's live pending revision.
func (s *Storage) Release(ctx context.Context, key, token string, now time.Time) error {
	return s.transition(ctx, key, token, now, func(rev uint64) error {
		return s.KV().Purge(ctx, key, jetstream.LastRevision(rev))
	})
}

func (s *Storage) transition(ctx context.Context, key, token string, now time.Time, write func(uint64) error) error {
	if err := s.observe(ctx, now); err != nil {
		return err
	}
	for {
		current, rev, err := s.latest(ctx, key)
		if err != nil {
			return err
		}
		if rev == 0 || current.State != requestid.StatePending || current.Token != token || !now.Before(current.ExpiresAt) {
			return requestid.ErrReservationLost
		}
		err = write(rev)
		if !errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
			return coreerrs.WrapOperation(err, "update owned request ID")
		}
	}
}
