// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
)

func BenchmarkReserveLive(b *testing.B) {
	_, js := natstest.ConnectJetStream(b, natstest.StartNATSServer(b))
	s := open(b, js)
	key, now := requestidtest.Key(b.Name()), requestidtest.Base
	entry := requestid.Entry{State: requestid.StatePending, Token: "owner", ExpiresAt: now.Add(time.Hour)}
	_, _, err := s.Reserve(b.Context(), key, entry, now)
	require.NoError(b, err)
	for b.Loop() {
		_, _, err := s.Reserve(b.Context(), key, entry, now)
		require.NoError(b, err)
	}
}

func BenchmarkReserveAndComplete(b *testing.B) {
	_, js := natstest.ConnectJetStream(b, natstest.StartNATSServer(b))
	s := open(b, js)
	key, now := requestidtest.Key(b.Name()), requestidtest.Base
	for b.Loop() {
		pending := requestid.Entry{State: requestid.StatePending, Token: "owner", ExpiresAt: now.Add(time.Minute)}
		_, won, err := s.Reserve(b.Context(), key, pending, now)
		require.NoError(b, err)
		require.True(b, won)
		done := requestid.Entry{State: requestid.StateCompleted, ExpiresAt: now.Add(time.Hour)}
		require.NoError(b, s.Complete(b.Context(), key, pending.Token, done, now))
		now = done.ExpiresAt
	}
}

func BenchmarkReserveAndRelease(b *testing.B) {
	_, js := natstest.ConnectJetStream(b, natstest.StartNATSServer(b))
	s := open(b, js)
	key, now := requestidtest.Key(b.Name()), requestidtest.Base
	pending := requestid.Entry{State: requestid.StatePending, Token: "owner", ExpiresAt: now.Add(time.Minute)}
	for b.Loop() {
		_, won, err := s.Reserve(b.Context(), key, pending, now)
		require.NoError(b, err)
		require.True(b, won)
		require.NoError(b, s.Release(b.Context(), key, pending.Token, now))
	}
}
