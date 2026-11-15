// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsbase_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/nats/natsbase"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
)

func TestNewBaseWithBucket_CreatesAndAdoptsBucket(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	cfg := natskvlease.BucketSpec{Bucket: "base-cov", TTL: time.Hour, Storage: jetstream.MemoryStorage, Replicas: 1}
	logger := slog.New(slog.DiscardHandler)

	base, err := natsbase.NewBaseWithBucket(t.Context(), js, cfg, logger)
	require.NoError(t, err)
	require.Equal(t, "base-cov", base.KV().Bucket())
	_, err = base.KV().Put(t.Context(), "k", []byte("v"))
	require.NoError(t, err)

	again, err := natsbase.NewBaseWithBucket(t.Context(), js, cfg, logger)
	require.NoError(t, err)
	entry, err := again.KV().Get(t.Context(), "k")
	require.NoError(t, err, "an existing bucket is adopted, not recreated")
	require.Equal(t, []byte("v"), entry.Value())
}

func TestNewBaseWithBucket_PropagatesMismatch(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	logger := slog.New(slog.DiscardHandler)
	cfg := natskvlease.BucketSpec{Bucket: "base-ttl", TTL: time.Hour, Storage: jetstream.MemoryStorage, Replicas: 1}
	_, err := natsbase.NewBaseWithBucket(t.Context(), js, cfg, logger)
	require.NoError(t, err)

	cfg.TTL = 2 * time.Hour
	_, err = natsbase.NewBaseWithBucket(t.Context(), js, cfg, logger)
	require.ErrorIs(t, err, natskvlease.ErrBucketTTLMismatch)
}
