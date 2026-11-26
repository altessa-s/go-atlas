// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go tool optgen generate

import (
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/appinfo"
)

// DefaultMaxAge is the default TTL for keys (24 hours).
const DefaultMaxAge = 24 * time.Hour

// DefaultReplicas is the default replica count.
const DefaultReplicas = 1

// options contains NATS JetStream idempotency storage configuration.
type options struct {
	bucket   string        `optgen:"default=defaultBucket()"`
	maxAge   time.Duration `optgen:"default=DefaultMaxAge"`
	replicas int           `optgen:"default=DefaultReplicas"`

	// migrateBucketTTL updates a pre-existing bucket whose key TTL differs
	// from the configured one instead of rejecting it with
	// [ErrBucketTTLMismatch]. It also enables per-key TTL on an existing
	// bucket, even when its key TTL already matches. Off by default:
	// the bucket's TTL governs keys of all processes sharing the bucket.
	migrateBucketTTL bool
}

func defaultBucket() string {
	return appinfo.Name + "-idempotency"
}
