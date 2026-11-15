// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

//go:generate go tool optgen generate

import (
	"context"
	"log/slog"
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
	// WithLogger sets the logger [Store.Watch] reports failed capability
	// probes and change-stream reconnects on. Nothing is logged by default.
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
}
