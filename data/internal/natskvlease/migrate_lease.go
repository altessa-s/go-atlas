// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// ErrBucketMigrationLocked reports that another storage migration of the
// bucket holds its lease — it is running, or crashed less than a lease TTL ago
// — or that this migration lost its lease and stopped.
var ErrBucketMigrationLocked = errors.New("KeyValue bucket storage migration is locked by another migrator")

// ErrMigrationLeaseStore reports that the lease store bucket
// (kvmigrate_leases) exists but is not one the migration created, or has a
// configuration that could expire or evict a live lease.
var ErrMigrationLeaseStore = errors.New("storage migration lease store is not usable")

// DefaultMigrationLeaseTTL is the lifetime of a migration lease without
// renewal. A running migration renews it every third of that; a crashed one
// releases the bucket for a Resume once it has passed.
const DefaultMigrationLeaseTTL = 30 * time.Second

const (
	migrationLeaseBucket   = "kvmigrate_leases"
	maxMigrationLeaseTTL   = 10 * time.Minute
	roleLeases             = "leases"
	leaseRenewEveryEntries = 100
	leaseRenewsPerTTL      = 3
	leaseReleaseTimeout    = 5 * time.Second
)

// isReservedBucket reports bucket names the migration uses itself.
func isReservedBucket(bucket string) bool {
	return bucket == migrationLeaseBucket || strings.HasPrefix(bucket, migrationTemplatePrefix)
}

// leaseOwner is the value of a lease: who holds it, for diagnostics.
type leaseOwner struct {
	ID      string    `json:"id"`
	Host    string    `json:"host"`
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
}

// migrationLease is this migrator's hold on a bucket. Renewing it is a
// compare-and-set on its revision, so a migrator whose lease expired or was
// taken over learns so at its next renewal and stops.
type migrationLease struct {
	kv    jetstream.KeyValue
	key   string
	value []byte
	ttl   time.Duration

	mu  sync.Mutex
	rev uint64

	stopHeartbeat context.CancelFunc
	heartbeatDone chan struct{}
}

// acquireLease takes the migration lease of bucket or fails with
// ErrBucketMigrationLocked when another migrator holds it.
func (h *KVHelper) acquireLease(ctx context.Context, bucket string) (*migrationLease, error) {
	kv, ttl, err := h.leaseStore(ctx)
	if err != nil {
		return nil, err
	}

	id, err := newMigrationID()
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	value, err := json.Marshal(leaseOwner{ID: id, Host: host, PID: os.Getpid(), Started: time.Now().UTC()})
	if err != nil {
		return nil, err
	}

	rev, err := kv.Create(ctx, bucket, value)
	if errors.Is(err, jetstream.ErrKeyExists) {
		holder := "another migrator"
		if entry, gerr := kv.Get(ctx, bucket); gerr == nil {
			var owner leaseOwner
			if json.Unmarshal(entry.Value(), &owner) == nil && owner.ID != "" {
				holder = fmt.Sprintf("migrator %s (host %s, pid %d)", owner.ID, owner.Host, owner.PID)
			}
			holder += fmt.Sprintf(", renewed %s ago", time.Since(entry.Created()).Round(time.Second))
		}
		return nil, fmt.Errorf("%w: bucket %q is held by %s; it is released when that run ends or its lease expires (%s)",
			ErrBucketMigrationLocked, bucket, holder, ttl)
	}
	if err != nil {
		return nil, fmt.Errorf("acquire migration lease: %w", err)
	}
	return &migrationLease{kv: kv, key: bucket, value: value, ttl: ttl, rev: rev}, nil
}

// leaseStore opens the lease bucket, creating it on first use, and refuses one
// that could lose or evict a live lease.
func (h *KVHelper) leaseStore(ctx context.Context) (jetstream.KeyValue, time.Duration, error) {
	name := kvStreamName(migrationLeaseBucket)
	info, err := h.streamInfo(ctx, name)
	if err != nil {
		return nil, 0, err
	}
	if info == nil {
		ttl := h.leaseTTL
		if ttl <= 0 {
			ttl = DefaultMigrationLeaseTTL
		}
		_, err = h.js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:   migrationLeaseBucket,
			History:  1,
			TTL:      ttl,
			Storage:  jetstream.FileStorage,
			Metadata: map[string]string{metaRole: roleLeases},
		})
		if err != nil && !errors.Is(err, jetstream.ErrBucketExists) {
			return nil, 0, fmt.Errorf("create migration lease store: %w", err)
		}
		if info, err = h.streamInfo(ctx, name); err != nil {
			return nil, 0, err
		}
		if info == nil {
			return nil, 0, fmt.Errorf("%w: %s disappeared after creation", ErrMigrationLeaseStore, migrationLeaseBucket)
		}
	}

	c := info.Config
	switch {
	case c.Metadata[metaRole] != roleLeases:
		return nil, 0, fmt.Errorf("%w: %s is not a migration lease store", ErrMigrationLeaseStore, migrationLeaseBucket)
	case c.Storage != jetstream.FileStorage:
		return nil, 0, fmt.Errorf("%w: %s must use file storage", ErrMigrationLeaseStore, migrationLeaseBucket)
	case c.MaxAge <= 0 || c.MaxAge > maxMigrationLeaseTTL:
		return nil, 0, fmt.Errorf("%w: %s key TTL %s is outside (0, %s]", ErrMigrationLeaseStore, migrationLeaseBucket, c.MaxAge, maxMigrationLeaseTTL)
	case c.MaxMsgsPerSubject != 1 || c.Retention != jetstream.LimitsPolicy:
		return nil, 0, fmt.Errorf("%w: %s must keep one message per key under limits retention", ErrMigrationLeaseStore, migrationLeaseBucket)
	case (c.MaxMsgs > 0 || c.MaxBytes > 0) && c.Discard != jetstream.DiscardNew:
		return nil, 0, fmt.Errorf("%w: %s could evict a live lease (global limits with discard old)", ErrMigrationLeaseStore, migrationLeaseBucket)
	case c.Mirror != nil || len(c.Sources) > 0 || c.RePublish != nil:
		return nil, 0, fmt.Errorf("%w: %s must be a standalone bucket", ErrMigrationLeaseStore, migrationLeaseBucket)
	}

	kv, err := h.js.KeyValue(ctx, migrationLeaseBucket)
	if err != nil {
		return nil, 0, fmt.Errorf("open migration lease store: %w", err)
	}
	return kv, c.MaxAge, nil
}

// renew extends the lease if it is still ours. Every mutating step of the
// migration calls it first, so a migrator that lost its lease stops before
// mutating anything.
func (l *migrationLease) renew(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	rev, err := l.kv.Update(ctx, l.key, l.value, l.rev)
	if err != nil {
		return fmt.Errorf("%w: lease of bucket %q lost: %v", ErrBucketMigrationLocked, l.key, err)
	}
	l.rev = rev
	return nil
}

// startHeartbeat renews the lease every third of its TTL until stopped, and
// cancels the migration with the renewal error if the lease is lost.
func (l *migrationLease) startHeartbeat(ctx context.Context, cancel context.CancelCauseFunc) {
	hbCtx, stop := context.WithCancel(ctx)
	l.stopHeartbeat = stop
	l.heartbeatDone = make(chan struct{})
	go func() {
		defer close(l.heartbeatDone)
		ticker := time.NewTicker(l.ttl / leaseRenewsPerTTL)
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				if err := l.renew(hbCtx); err != nil {
					if hbCtx.Err() == nil {
						cancel(err)
					}
					return
				}
			}
		}
	}()
}

// release stops the heartbeat and deletes the lease if it is still ours. If
// the deletion fails the lease expires on its own. ctx should not be canceled
// by the migration's own failure, so that a failed run still releases.
func (l *migrationLease) release(ctx context.Context) {
	if l.stopHeartbeat != nil {
		l.stopHeartbeat()
		<-l.heartbeatDone
	}
	ctx, cancel := context.WithTimeout(ctx, leaseReleaseTimeout)
	defer cancel()
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.kv.Delete(ctx, l.key, jetstream.LastRevision(l.rev))
}
