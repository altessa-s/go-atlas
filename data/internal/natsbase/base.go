// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsbase

import (
	"context"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
)

// Base provides common functionality for NATS JetStream KeyValue providers.
// Embed this type in provider structs to get access to the underlying KeyValue store.
type Base struct {
	kv jetstream.KeyValue
}

// NewBase creates a new Base with an existing KeyValue store.
// Use this when the bucket is already created by the caller.
func NewBase(kv jetstream.KeyValue) Base {
	return Base{kv: kv}
}

// NewBaseWithBucket creates a new Base by creating or getting a bucket using JetStream.
// The bucket will be created with the specified configuration if it doesn't exist.
func NewBaseWithBucket(ctx context.Context, js jetstream.JetStream, cfg natskvlease.BucketConfig, logger *slog.Logger) (Base, error) {
	kvHelper := natskvlease.NewKVHelper(js, logger)
	kv, err := kvHelper.GetOrCreateBucket(ctx, cfg)
	if err != nil {
		return Base{}, err
	}
	return Base{kv: kv}, nil
}

// KV returns the underlying KeyValue store for direct access.
func (b *Base) KV() jetstream.KeyValue {
	return b.kv
}
