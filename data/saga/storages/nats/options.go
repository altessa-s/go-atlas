// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import "time"

const (
	// DefaultBucket is the name of the KeyValue bucket holding saga instances.
	DefaultBucket = "saga"

	// DefaultBucketTTL is the bucket-level time-to-live for saga instances. It
	// is a long backstop, not the primary lifecycle: active sagas rewrite their
	// key on every checkpoint (resetting the TTL), completed sagas are deleted
	// explicitly, and stalled ones are rolled back by the recovery cycle well
	// inside this window. Tune it to your longest expected saga lifetime plus a
	// retention margin.
	DefaultBucketTTL = 30 * 24 * time.Hour
)

// options holds the NATS KeyValue store configuration.
type options struct {
	bucket    string        `optgen:"default=DefaultBucket"`
	bucketTTL time.Duration `optgen:"default=DefaultBucketTTL"`

	// migrateBucketTTL updates a pre-existing bucket whose key TTL differs
	// from the configured one instead of rejecting it with
	// [ErrBucketTTLMismatch]. Off by default: the bucket's TTL governs every
	// key in it, including keys of other processes sharing the bucket.
	migrateBucketTTL bool
}
