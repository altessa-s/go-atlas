// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
)

// lockMetadata represents the metadata stored with each lock
type lockMetadata struct {
	OwnerId     string    `json:"owner_id"`
	AcquiredAt  time.Time `json:"acquired_at"`
	LastRenewed time.Time `json:"last_renewed"`
	TTL         int64     `json:"ttl"`
}

type lock struct {
	isLocked atomic.Bool
	lease    *natskvlease.Lease
	config   *config
	logger   *slog.Logger

	metadata *lockMetadata
}

func newLock(cfg *config, kvOps *natskvlease.KVOps, logger *slog.Logger, renewRatio float64) *lock {
	metadata := &lockMetadata{
		OwnerId:     uuid.NewString(),
		AcquiredAt:  time.Now(),
		LastRenewed: time.Now(),
		TTL:         int64(cfg.TTL),
	}

	metadataBytes, _ := json.Marshal(metadata) //nolint:errcheck

	l := &lock{
		config:   cfg,
		logger:   logger,
		metadata: metadata,
	}

	l.lease = natskvlease.NewLease(kvOps.KV(), natskvlease.LeaseConfig{
		Key:        cfg.Key,
		TTL:        cfg.TTL,
		RenewRatio: renewRatio,
		Value:      metadataBytes,
		IsOwner: func(value []byte) bool {
			var m lockMetadata
			if err := json.Unmarshal(value, &m); err != nil {
				return false
			}
			return m.OwnerId == metadata.OwnerId
		},
		Callbacks: natskvlease.LeaseCallbacks{
			OnAcquired: func() { l.isLocked.Store(true) },
			OnLost:     func() { l.isLocked.Store(false) },
			OnReleased: func() { l.isLocked.Store(false) },
			OnRenewed: func() {
				l.metadata.LastRenewed = time.Now()
				// Update value with new timestamp for next renewal visibility
				if b, err := json.Marshal(l.metadata); err == nil {
					l.lease.UpdateValue(b)
				}
			},
		},
		Logger: logger,
	})

	return l
}

// Release releases the lock with context support.
func (l *lock) Release(ctx context.Context) error {
	return l.lease.Release(ctx)
}

// IsLocked returns true if the lock is currently held by this instance.
func (l *lock) IsLocked() bool {
	return l.isLocked.Load()
}

// GetLockInfo returns information about the current state of the lock.
func (l *lock) GetLockInfo(ctx context.Context) (*providers.LockInfo, error) {
	entry, err := l.lease.GetOps().Get(ctx, l.config.Key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, errs.ErrLockNotHeld
		}
		return nil, err
	}

	// Check if entry or its value is nil (defensive check for tombstones)
	if entry == nil || entry.Value() == nil {
		return nil, errs.ErrLockNotHeld
	}

	var metadata lockMetadata
	if err := json.Unmarshal(entry.Value(), &metadata); err != nil {
		return nil, err
	}

	// Best-effort staleness hint — see [providers.LockInfo.IsStale] for
	// the caveats. The subtraction below is wall-clock-based and may
	// disagree between nodes; safety-critical decisions belong to the
	// FencingToken / KV TTL path, never to this flag.
	now := time.Now()
	isStale := now.Sub(metadata.LastRenewed) > time.Duration(metadata.TTL)

	return &providers.LockInfo{
		Key:          l.config.Key,
		Owner:        metadata.OwnerId,
		AcquiredAt:   metadata.AcquiredAt,
		LastRenewed:  metadata.LastRenewed,
		TTL:          time.Duration(metadata.TTL),
		IsStale:      isStale,
		FencingToken: entry.Revision(),
	}, nil
}

// run acquires the lock and leaves the lease renewing in the background.
// ctx scopes the lock; acquireTimeout bounds only the attempt — see
// [natskvlease.Lease.RunCamping].
func (l *lock) run(ctx context.Context, acquireTimeout time.Duration) (bool, error) {
	return l.lease.RunCamping(ctx, acquireTimeout)
}
