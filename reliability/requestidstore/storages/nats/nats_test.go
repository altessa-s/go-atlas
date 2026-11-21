// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/reliability/requestidstore/storages/nats"
)

func open(tb testing.TB, js jetstream.JetStream, opts ...nats.Option) *nats.Storage {
	tb.Helper()
	store, err := nats.New(tb.Context(), js, append(opts, nats.WithBucket("test_"+requestidtest.Key(tb.Name())))...)
	require.NoError(tb, err)
	tb.Cleanup(func() { require.NoError(tb, store.Close(context.WithoutCancel(tb.Context()))) })
	return store
}

func TestConformance(t *testing.T) {
	t.Parallel()
	_, js := natstest.ConnectJetStream(t, natstest.StartNATSServer(t))
	requestidtest.Run(t, func(t *testing.T) requestid.Store { return open(t, js) })
}

func TestPurgeUsesCallerTime(t *testing.T) {
	t.Parallel()
	_, js := natstest.ConnectJetStream(t, natstest.StartNATSServer(t))
	s := open(t, js, nats.WithPurgeInterval(time.Millisecond))
	now := requestidtest.Base
	key := requestidtest.Key(t.Name())
	entry := requestid.Entry{State: requestid.StatePending, Token: string([]byte{0xff}), ExpiresAt: now.Add(time.Hour)}
	_, won, err := s.Reserve(t.Context(), key, entry, now)
	require.NoError(t, err)
	require.True(t, won)
	// Also check round-trip of tokens that are not UTF-8.
	seen, won, err := s.Reserve(t.Context(), key, entry, now)
	require.NoError(t, err)
	require.False(t, won)
	require.Equal(t, entry.Token, seen.Token)
	err = s.Release(t.Context(), requestidtest.Key("absent"), "other", entry.ExpiresAt)
	require.ErrorIs(t, err, requestid.ErrReservationLost)
	require.Eventually(t, func() bool {
		_, getErr := s.KV().Get(t.Context(), key)
		return errors.Is(getErr, jetstream.ErrKeyNotFound)
	}, time.Second*5, time.Millisecond*5)
}

func TestBucketTTLRejected(t *testing.T) {
	t.Parallel()
	_, js := natstest.ConnectJetStream(t, natstest.StartNATSServer(t))
	_, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{Bucket: "expiring", TTL: time.Hour})
	require.NoError(t, err)
	_, err = nats.New(t.Context(), js, nats.WithBucket("expiring"))
	require.ErrorIs(t, err, nats.ErrBucketTTLMismatch)
}
