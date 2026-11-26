// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	limitnats "github.com/altessa-s/go-atlas/coord/limiters/storages/nats"
)

func TestProvider_RejectsLimitAbovePayloadCapacity(t *testing.T) {
	t.Parallel()
	const maxPayload = 2048
	ns, err := server.NewServer(&server.Options{
		Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, JetStream: true,
		StoreDir: t.TempDir(), MaxPayload: maxPayload,
	})
	require.NoError(t, err)
	ns.Start()
	t.Cleanup(func() { ns.Shutdown(); ns.WaitForShutdown() })
	require.True(t, ns.ReadyForConnections(5*time.Second))
	nc, js := natstest.ConnectJetStream(t, ns)
	require.EqualValues(t, maxPayload, nc.MaxPayload())
	p, err := limitnats.New(t.Context(), js, limitnats.WithBucket("small-payload"))
	require.NoError(t, err)
	// Budget for a signed timestamp plus a separator per entry, the
	// JSON envelope with three numeric fields, and the revision header.
	const timestamp = "-9223372036854775808"
	const envelope = `{"requests":[],"limit":` + timestamp +
		`,"window":` + timestamp + `,"lastUsed":` + timestamp + `}`
	const header = "NATS/1.0\r\nNats-Expected-Last-Subject-Sequence: 18446744073709551615\r\n\r\n"
	const capacity = int64((maxPayload - len(header) - len(envelope)) / len(timestamp+","))

	info, err := p.Allow(t.Context(), "large-quota", capacity+1, time.Hour)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	require.Nil(t, info)
	_, err = p.KV().Get(t.Context(), "large-quota")
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound)

	for range capacity {
		_, err := p.Allow(t.Context(), "fits", capacity, time.Hour)
		require.NoError(t, err)
	}
	_, err = p.Allow(t.Context(), "fits", capacity, time.Hour)
	require.ErrorIs(t, err, limiters.ErrLimitExceeded)
}
