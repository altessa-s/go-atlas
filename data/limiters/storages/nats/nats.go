// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/internal/natsbase"
	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
	"github.com/altessa-s/go-atlas/data/limiters/storages"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// bucketData represents the rate limiting data stored in NATS KeyValue.
type bucketData struct {
	Requests []int64 `json:"requests"` // Unix timestamps in milliseconds
	Limit    int64   `json:"limit"`    // Rate limit
	Window   int64   `json:"window"`   // Window duration in milliseconds
	LastUsed int64   `json:"lastUsed"` // Last access time in milliseconds
}

// Provider implements a NATS JetStream KeyValue-based rate limiting provider using sliding window algorithm.
type Provider struct {
	natsbase.Base
	options *options
}

// Ensure Provider implements Storage interface
var _ storages.Storage = (*Provider)(nil)

// New creates a new NATS JetStream KeyValue rate limiting provider.
// Accepts a JetStream context and optional configuration options.
// Automatically creates the bucket if it doesn't exist.
//
// Returns an error if the JetStream context is nil or bucket creation fails.
//
// Example:
//
//	js, _ := jetstream.New(nc)
//	storage, err := nats.New(js, nats.WithBucket("rate-limiter"))
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer storage.Close()
func New(js jetstream.JetStream, opts ...Option) (*Provider, error) {
	if js == nil {
		return nil, fmt.Errorf("JetStream context cannot be nil")
	}

	config := newOptions(opts...)

	base, err := natsbase.NewBaseWithBucket(context.Background(), js, natskvlease.BucketConfig{
		Bucket:   config.bucket,
		TTL:      config.maxAge,
		Storage:  jetstream.FileStorage,
		Replicas: config.replicas,
	}, nil)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create NATS KeyValue bucket")
	}

	return &Provider{
		Base:    base,
		options: config,
	}, nil
}

// maxAllowCASAttempts caps the number of retries the read-modify-write loop
// in [Provider.Allow] performs on a NATS KV revision conflict. With per-call
// random jitter unlikely (the algorithm itself is deterministic), pure
// exponential contention beyond this limit usually indicates a hot key worth
// rejecting rather than retrying indefinitely.
const maxAllowCASAttempts = 5

// Allow checks if a request should be allowed based on the rate limit using sliding window algorithm.
//
// The read-modify-write cycle uses NATS KeyValue revision-based CAS:
// [jetstream.KeyValue.Create] for the first write to a key and
// [jetstream.KeyValue.Update] with the entry revision for subsequent writes.
// Concurrent updates that race on the same key are detected via
// [jetstream.ErrKeyExists] and retried up to [maxAllowCASAttempts] times,
// after which the call fails with a wrapped error rather than silently
// dropping a competing update — losing a write here would let a burst slip
// past the limit.
func (p *Provider) Allow(ctx context.Context, key string, limit int64, period time.Duration) (*storages.LimitInfo, error) {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")
	panics.Must(limit > 0, "limit must be greater than 0")
	panics.Must(period > 0, "period must be greater than 0")

	windowMs := period.Milliseconds()

	for range maxAllowCASAttempts {
		now := time.Now().UnixMilli()

		data, revision, err := p.loadBucket(ctx, key)
		if err != nil {
			return nil, err
		}

		// Update bucket parameters.
		data.Limit = limit
		data.Window = windowMs
		data.LastUsed = now

		// Remove expired requests (sliding window).
		cutoff := now - windowMs
		filteredSeq := coreslices.Filter(data.Requests, func(reqTime int64) bool {
			return reqTime > cutoff
		})
		data.Requests = slices.Collect(filteredSeq)

		// Calculate reset time.
		resetTime := uint64(0)
		if len(data.Requests) > 0 {
			//nolint:mnd
			resetTime = uint64((data.Requests[0] + windowMs) / 1000) // #nosec G115 -- timestamps are positive
		}

		currentCount := int64(len(data.Requests))

		info := &storages.LimitInfo{
			Remaining: max(limit-currentCount, 0),
			Reset:     int64(resetTime), // #nosec G115 -- resetTime is Unix seconds, fits int64
		}

		if currentCount >= limit {
			// No write needed when the limit is already exceeded; no race to
			// guard against because we are not mutating shared state.
			return info, storages.ErrLimitExceeded
		}

		// Reserve a slot in our local copy.
		data.Requests = append(data.Requests, now)
		info.Remaining = max(limit-int64(len(data.Requests)), 0)

		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "marshal rate limit data")
		}

		if err := p.casPut(ctx, key, jsonData, revision); err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				// Another writer raced us. Re-read and recompute on the next
				// iteration; do not surface this as a user-visible error
				// unless we exhaust the retry budget.
				continue
			}
			return nil, coreerrs.WrapOperation(err, "store rate limit data in NATS KeyValue")
		}

		return info, nil
	}

	return nil, coreerrs.Wrap(errors.New("revision conflict"),
		"NATS rate limit CAS exhausted after concurrent updates")
}

// loadBucket fetches the current bucket entry and returns the parsed data
// plus the entry revision (0 when the key does not yet exist). The revision
// drives the CAS write in [Provider.Allow].
func (p *Provider) loadBucket(ctx context.Context, key string) (*bucketData, uint64, error) {
	entry, err := p.KV().Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return &bucketData{Requests: make([]int64, 0)}, 0, nil
		}
		return nil, 0, coreerrs.WrapOperation(err, "get rate limit data from NATS KeyValue")
	}

	data := &bucketData{}
	if err := json.Unmarshal(entry.Value(), data); err != nil {
		return nil, 0, coreerrs.WrapOperation(err, "unmarshal rate limit data")
	}
	return data, entry.Revision(), nil
}

// casPut writes the bucket back to NATS KV using revision-based CAS. When
// revision is 0 it issues [jetstream.KeyValue.Create] (succeeds only if no
// entry exists yet); otherwise it issues [jetstream.KeyValue.Update] (succeeds
// only when the entry still has the same revision the caller observed). Both
// surface a revision conflict as [jetstream.ErrKeyExists].
func (p *Provider) casPut(ctx context.Context, key string, value []byte, revision uint64) error {
	if revision == 0 {
		_, err := p.KV().Create(ctx, key, value)
		return err
	}
	_, err := p.KV().Update(ctx, key, value, revision)
	return err
}

// Reset resets the rate limit for a specific key by removing all stored requests.
func (p *Provider) Reset(ctx context.Context, key string) error {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")

	// Try to delete the key - ignore if it doesn't exist
	err := p.KV().Delete(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return coreerrs.WrapOperation(err, "reset rate limit data in NATS KeyValue")
	}

	return nil
}
