// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// DefaultBucket is the default NATS KeyValue bucket name.
const DefaultBucket = "uniq"

// DefaultTTL is the default TTL for the bucket keys.
const DefaultTTL = time.Hour * 24

// DefaultStorageType is the default storage type for the NATS bucket.
var DefaultStorageType = jetstream.MemoryStorage

// options contains NATS JetStream uniq provider configuration.
type options struct {
	bucket      string                `optgen:"default=DefaultBucket"`
	ttl         time.Duration         `optgen:"default=DefaultTTL"`
	storageType jetstream.StorageType `optgen:"default=DefaultStorageType"`
}
