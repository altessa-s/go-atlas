// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/retry"
	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/internal/natsbase"
	"github.com/altessa-s/go-atlas/data/internal/natskey"
	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
	"github.com/altessa-s/go-atlas/data/limiters/storages"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrBucketTTLMismatch is returned by New when the bucket already exists with a
// key TTL other than the configured one and [WithMigrateBucketTTL] is not set.
// The bucket is left untouched.
var ErrBucketTTLMismatch = natskvlease.ErrBucketTTLMismatch

// ErrBucketStorageMismatch is returned by New when the bucket already exists
// with another storage type and [WithStrictBucketStorage] is set. The bucket is
// left untouched.
var ErrBucketStorageMismatch = natskvlease.ErrBucketStorageMismatch

// ErrKeyTooLong is returned, before any network I/O, for a key whose escaped
// form exceeds the NATS KeyValue key budget (see the package README).
var ErrKeyTooLong = natskey.ErrKeyTooLong

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
	options   *options
	casPolicy *retry.Policy
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

	base, err := natsbase.NewBaseWithBucket(context.Background(), js, bucketConfig(config), nil)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create NATS KeyValue bucket")
	}

	return &Provider{
		Base:      base,
		options:   config,
		casPolicy: newCASPolicy(config.maxCASAttempts, config.maxCASBackoff),
	}, nil
}

// bucketConfig is the bucket New creates and MigrateBucketStorage migrates to.
func bucketConfig(o *options) natskvlease.BucketConfig {
	return natskvlease.BucketConfig{
		Bucket:        o.bucket,
		TTL:           o.maxAge,
		Storage:       jetstream.FileStorage,
		Replicas:      o.replicas,
		MigrateTTL:    o.migrateBucketTTL,
		StrictStorage: o.strictBucketStorage,
	}
}

// ErrContention is returned by [Provider.Allow] when other writers kept
// updating the key between every read and write of the call until its attempt
// budget ([WithMaxCASAttempts]) ran out. It is not a rate limit decision: the
// request was neither admitted nor counted, and the caller decides whether to
// fail open or closed. The error also wraps the last revision conflict
// ([jetstream.ErrKeyExists]).
var ErrContention = errors.New("NATS rate limit key too contended")

// casBaseBackoff is the ceiling of the first randomized pause after a
// revision conflict; it doubles per attempt up to the configured maximum.
const casBaseBackoff = time.Millisecond

// casMaxShift bounds the doubling of casBaseBackoff so the shift never
// overflows; 2^30 ms is far above any sensible maximum backoff.
const casMaxShift = 30

// newCASPolicy builds the retry policy of the read-modify-write loop in
// [Provider.Allow]: up to attempts attempts, retrying revision conflicts only,
// with full-jitter exponential backoff capped at maxBackoff. Full jitter
// spreads contenders that collided on one revision over the whole interval,
// so they do not collide again in lockstep.
func newCASPolicy(attempts int, maxBackoff time.Duration) *retry.Policy {
	return retry.NewPolicy(
		retry.WithMaxAttempts(attempts-1),
		retry.WithShouldRetry(func(err error) bool { return errors.Is(err, jetstream.ErrKeyExists) }),
		retry.WithNextDelay(func(attempt int, _ error) time.Duration {
			ceiling := min(maxBackoff, casBaseBackoff<<min(max(attempt, 0), casMaxShift))
			// A pause of at least 1ns: core/retry stops on a non-positive delay.
			return time.Duration(rand.Int64N(int64(ceiling))) + 1 // #nosec G404 -- jitter, not a secret
		}),
	)
}

// Allow checks if a request should be allowed based on the rate limit using sliding window algorithm.
//
// The read-modify-write cycle uses NATS KeyValue revision-based CAS:
// [jetstream.KeyValue.Create] for the first write to a key and
// [jetstream.KeyValue.Update] with the entry revision for subsequent writes.
// A write that loses a race on the same key ([jetstream.ErrKeyExists]) is
// never dropped — that would let a burst slip past the limit — but re-read,
// recomputed and retried after a randomized backoff, up to
// [WithMaxCASAttempts] attempts and as long as ctx allows. Rejections write
// nothing, so retrying can never admit more than limit requests. A call that
// exhausts its attempts returns [ErrContention].
func (p *Provider) Allow(ctx context.Context, key string, limit int64, period time.Duration) (*storages.LimitInfo, error) {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")
	panics.Must(limit > 0, "limit must be greater than 0")
	panics.Must(period > 0, "period must be greater than 0")

	key, err := physicalKey(key)
	if err != nil {
		return nil, err
	}

	// Windows are tracked in whole milliseconds; a sub-millisecond period
	// still gets a 1ms window rather than none, matching the Redis storage.
	windowMs := max(period.Milliseconds(), 1)

	var info *storages.LimitInfo
	err = p.casPolicy.Do(ctx, func(ctx context.Context) error {
		var attemptErr error
		info, attemptErr = p.tryAllow(ctx, key, limit, windowMs)
		return attemptErr
	})
	switch {
	case err == nil:
		return info, nil
	case errors.Is(err, storages.ErrLimitExceeded):
		return info, err
	case errors.Is(err, jetstream.ErrKeyExists):
		return nil, fmt.Errorf("%w after %d attempts: %w", ErrContention, p.options.maxCASAttempts, err)
	default:
		return nil, err
	}
}

// tryAllow makes one read-modify-write attempt of [Provider.Allow]. It returns
// [jetstream.ErrKeyExists] unwrapped when another writer updated the key
// since it was read.
func (p *Provider) tryAllow(ctx context.Context, key string, limit, windowMs int64) (*storages.LimitInfo, error) {
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

	// Reserve a slot in our local copy. Into an empty window it is the oldest
	// request, so the window frees its first slot when it expires.
	data.Requests = append(data.Requests, now)
	info.Remaining = max(limit-int64(len(data.Requests)), 0)
	if currentCount == 0 {
		info.Reset = (now + windowMs) / 1000 //nolint:mnd // milliseconds to Unix seconds
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "marshal rate limit data")
	}

	if err := p.casPut(ctx, key, jsonData, revision); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			// Another writer raced us: the retry policy re-reads and
			// recomputes on the next attempt.
			return nil, err
		}
		return nil, coreerrs.WrapOperation(err, "store rate limit data in NATS KeyValue")
	}

	return info, nil
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

	key, err := physicalKey(key)
	if err != nil {
		return err
	}

	// Try to delete the key - ignore if it doesn't exist
	err = p.KV().Delete(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return coreerrs.WrapOperation(err, "reset rate limit data in NATS KeyValue")
	}

	return nil
}

// physicalKey maps a logical rate-limit key (an IPv6 address, a
// "service:recognizer" budget key, a token) onto the NATS KeyValue key it is
// stored under. Allow and Reset apply it once, so both reach the same entry.
func physicalKey(key string) (string, error) {
	physical, err := natskey.Encode(key)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "encode NATS KeyValue key")
	}
	return physical, nil
}
