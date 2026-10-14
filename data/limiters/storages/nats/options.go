// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

const (
	// DefaultMaxAge is the default maximum age for keys in the NATS KeyValue bucket.
	// Rate limiting data older than this duration will be automatically purged by NATS.
	DefaultMaxAge = 24 * time.Hour

	// DefaultReplicas is the default number of replicas for the KeyValue bucket.
	DefaultReplicas = 1

	// DefaultMaxCASAttempts is the default number of read-modify-write
	// attempts [Provider.Allow] makes on one call before it gives up on a
	// key other writers keep updating and returns [ErrContention].
	DefaultMaxCASAttempts = 64

	// DefaultMaxCASBackoff is the default ceiling of the randomized pause
	// between two attempts of [Provider.Allow] after a revision conflict.
	DefaultMaxCASBackoff = 20 * time.Millisecond
)

// options contains NATS rate limiting storage provider configuration.
type options struct {
	// Bucket is the NATS JetStream KeyValue bucket name used for storing rate limiting data.
	// Default is "rate-limiter".
	bucket string `optgen:"default=defaultBucket()"`

	// MaxAge is the maximum age for keys in the bucket (global TTL).
	// This acts as a global TTL for all rate limiting data in the bucket.
	// Default is 24 hours.
	maxAge time.Duration `optgen:"default=DefaultMaxAge" optcheck:"nonempty"`

	// Replicas is the number of replicas for the KeyValue bucket.
	// Higher replica counts provide better availability but increase storage overhead.
	// Default is 1.
	replicas int `optgen:"default=DefaultReplicas" optcheck:"nonempty"`

	// maxCASAttempts caps the read-modify-write attempts of one Allow call
	// on a key other callers update concurrently. A call exhausting it
	// returns [ErrContention]. Non-positive values keep the default.
	maxCASAttempts int `optgen:"default=DefaultMaxCASAttempts" optval:"positive"`

	// maxCASBackoff caps the randomized pause between two attempts after a
	// revision conflict: the n-th pause is drawn uniformly from
	// (0, min(maxCASBackoff, 1ms * 2^n)]. Non-positive values keep the default.
	maxCASBackoff time.Duration `optgen:"default=DefaultMaxCASBackoff" optcheck:"nonempty"`

	// migrateBucketTTL updates a pre-existing bucket whose key TTL differs
	// from the configured one instead of rejecting it with
	// [ErrBucketTTLMismatch]. Off by default: the bucket's TTL governs every
	// key in it, including keys of other processes sharing the bucket.
	migrateBucketTTL bool

	// strictBucketStorage rejects a pre-existing bucket whose storage type
	// differs from the one this backend asks for with
	// [ErrBucketStorageMismatch] instead of adopting it with a warning.
	strictBucketStorage bool
}

func defaultBucket() string {
	return appinfo.Name + "-rate-limiter"
}
