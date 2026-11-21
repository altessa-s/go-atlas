// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
)

// beforePurge interleaves a takeover after the scan has seen the old revision.
type beforePurge struct {
	jetstream.Stream
	replace func()
}

func (s beforePurge) Purge(ctx context.Context, opts ...jetstream.StreamPurgeOpt) error {
	s.replace()
	return s.Stream.Purge(ctx, opts...)
}

func TestPurgeKeepsReplacement(t *testing.T) {
	t.Parallel()
	_, js := natstest.ConnectJetStream(t, natstest.StartNATSServer(t))
	s, err := New(t.Context(), js, WithPurgeInterval(0))
	require.NoError(t, err)
	key, now := requestidtest.Key(t.Name()), requestidtest.Base
	first := requestid.Entry{State: requestid.StatePending, Token: "first", ExpiresAt: now.Add(time.Minute)}
	_, won, err := s.Reserve(t.Context(), key, first, now)
	require.NoError(t, err)
	require.True(t, won)
	now = first.ExpiresAt
	s.clock.Observe(now)
	replacement := requestid.Entry{State: requestid.StatePending, Token: "replacement", ExpiresAt: now.Add(time.Hour)}
	s.stream = beforePurge{Stream: s.stream, replace: func() {
		_, reserved, reserveErr := s.Reserve(t.Context(), key, replacement, now)
		require.NoError(t, reserveErr)
		require.True(t, reserved)
	}}
	count, err := s.purgeExpired(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	got, won, err := s.Reserve(t.Context(), key, first, now)
	require.NoError(t, err)
	require.False(t, won)
	require.Equal(t, replacement.Token, got.Token)
}
