// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"context"
	"time"
)

// DefaultCollectionName is the default MongoDB collection name for outbox events.
const DefaultCollectionName = "events_outbox"

// DefaultIndexCreateTimeout is the default timeout for initial index creation.
const DefaultIndexCreateTimeout = 10 * time.Second

type options struct {
	collectionName string          `optval:"nonempty" optgen:"default=DefaultCollectionName"`
	ctx            context.Context `opt:"Context" optgen:"notnil"`
	indexTimeout   time.Duration   `opt:"IndexCreateTimeout" optval:"positive" optgen:"default=DefaultIndexCreateTimeout"`
}
