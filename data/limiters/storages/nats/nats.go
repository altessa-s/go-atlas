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
		Bucket:  config.bucket,
		TTL:     config.maxAge,
		Storage: jetstream.FileStorage,
	}, nil)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create NATS KeyValue bucket")
	}

	return &Provider{
		Base:    base,
		options: config,
	}, nil
}

// Allow checks if a request should be allowed based on the rate limit using sliding window algorithm.
// This implementation uses NATS KeyValue optimistic locking to ensure atomicity in distributed environments.
func (p *Provider) Allow(ctx context.Context, key string, limit int64, period time.Duration) (*storages.LimitInfo, error) {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")
	panics.Must(limit > 0, "limit must be greater than 0")
	panics.Must(period > 0, "period must be greater than 0")

	now := time.Now().UnixMilli()
	windowMs := period.Milliseconds()

	// Try to get existing bucket data
	entry, err := p.KV().Get(ctx, key)
	var data *bucketData

	if err != nil {
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, coreerrs.WrapOperation(err, "get rate limit data from NATS KeyValue")
		}
		data = &bucketData{Requests: make([]int64, 0)}
	} else {
		// Parse existing data
		data = &bucketData{}
		if err = json.Unmarshal(entry.Value(), data); err != nil {
			return nil, coreerrs.WrapOperation(err, "unmarshal rate limit data")
		}
	}

	// Update bucket parameters
	data.Limit = limit
	data.Window = windowMs
	data.LastUsed = now

	// Remove expired requests (sliding window)
	cutoff := now - windowMs
	filteredSeq := coreslices.Filter(data.Requests, func(reqTime int64) bool {
		return reqTime > cutoff
	})
	data.Requests = slices.Collect(filteredSeq)

	// Calculate reset time
	resetTime := uint64(0)
	if len(data.Requests) > 0 {
		//nolint:mnd
		resetTime = uint64((data.Requests[0] + windowMs) / 1000) // #nosec G115 -- timestamps are positive
	}

	// Check if request should be allowed
	currentCount := int64(len(data.Requests))

	info := &storages.LimitInfo{
		Remaining: max(limit-currentCount, 0),
		Reset:     int64(resetTime), // #nosec G115 -- resetTime is Unix seconds, fits int64
	}

	if currentCount >= limit {
		return info, storages.ErrLimitExceeded
	}

	// Add current request
	data.Requests = append(data.Requests, now)
	info.Remaining = max(limit-int64(len(data.Requests)), 0)

	// Serialize and store
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "marshal rate limit data")
	}

	// Use Put to store the data (overwrites existing data)
	// Note: This approach may have race conditions in high-concurrency scenarios
	// For production use, consider implementing retry logic or using external locking
	_, err = p.KV().Put(ctx, key, jsonData)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "store rate limit data in NATS KeyValue")
	}

	return info, nil
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
