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
}

func defaultBucket() string {
	return appinfo.Name + "-rate-limiter"
}
