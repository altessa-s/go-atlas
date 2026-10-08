// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"context"
	"time"
)

// DefaultCollectionName is the collection [New] stores keys in.
const DefaultCollectionName = "idempotency_keys"

// DefaultTTL is how long a key lives after its last write (24 hours).
const DefaultTTL = 24 * time.Hour

// DefaultIndexCreateTimeout bounds the TTL index creation in [New].
const DefaultIndexCreateTimeout = 10 * time.Second

// options contains MongoDB idempotency storage configuration.
type options struct {
	collectionName string          `optval:"nonempty" optgen:"default=DefaultCollectionName"`
	ttl            time.Duration   `optgen:"default=DefaultTTL"`
	ctx            context.Context `opt:"Context" optgen:"notnil"`
	indexTimeout   time.Duration   `opt:"IndexCreateTimeout" optval:"positive" optgen:"default=DefaultIndexCreateTimeout"`
}
