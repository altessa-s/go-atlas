// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/coord/limiters/budget"
	"github.com/altessa-s/go-atlas/coord/limiters/tokenbucket"
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

func TestProvider_AdoptedBucketPayloadCapacity(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	nc, js := natstest.ConnectJetStream(t, ns)
	const maxValueSize = 2048
	require.Greater(t, nc.MaxPayload(), int64(maxValueSize))
	kv, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{
		Bucket: "small-value", TTL: limitnats.DefaultMaxAge,
		Storage: jetstream.FileStorage, MaxValueSize: maxValueSize,
	})
	require.NoError(t, err)
	p, err := limitnats.New(t.Context(), js, limitnats.WithBucket(kv.Bucket()))
	require.NoError(t, err)

	const quota = 1000
	for _, tc := range []struct {
		name  string
		check func() error
	}{
		{"budget", func() error {
			_, err := budget.New(quota, time.Hour, p)
			return err
		}},
		{"token-bucket default", func() error {
			_, err := tokenbucket.New(tokenbucket.RateLimitSettings{Limit: quota, Period: time.Hour}, p)
			return err
		}},
		{"token-bucket target", func() error {
			_, err := tokenbucket.New(tokenbucket.RateLimitSettings{Limit: 1, Period: time.Hour}, p,
				tokenbucket.WithRules(&tokenbucket.RateLimitRule{Target: "192.0.2.1",
					RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: quota, Period: time.Hour}}))
			return err
		}},
		{"direct admission", func() error {
			_, err := p.Allow(t.Context(), "large-quota", quota, time.Hour)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, tc.check(), coreerrs.ErrInvalidArgument)
		})
	}
	_, err = kv.Get(t.Context(), "large-quota")
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound)
	_, err = budget.New(1, time.Hour, p)
	require.NoError(t, err)
	_, err = p.Allow(t.Context(), "fits", 1, time.Hour)
	require.NoError(t, err)
}

type unknownPayloadJetStream struct{ jetstream.JetStream }

func (unknownPayloadJetStream) Conn() *nats.Conn { return &nats.Conn{} }

func TestNew_RejectsUnknownMaxPayload(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	_, err := limitnats.New(t.Context(), unknownPayloadJetStream{js}, limitnats.WithBucket("unknown-payload"))
	require.ErrorContains(t, err, "NATS MaxPayload is not yet known")
}
