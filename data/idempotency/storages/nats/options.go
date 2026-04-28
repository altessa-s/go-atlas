// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
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
}

func defaultBucket() string {
	return appinfo.Name + "-idempotency"
}
