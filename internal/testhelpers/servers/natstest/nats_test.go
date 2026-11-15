// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natstest_test

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
)

func TestNATSServerCleanup(t *testing.T) {
	t.Parallel()
	var dir string
	t.Run("file backed consumer", func(t *testing.T) {
		// The parent checks cleanup after this subtest returns.
		ns := natstest.StartNATSServer(t)
		dir = ns.JetStreamConfig().StoreDir
		_, js := natstest.ConnectJetStream(t, ns)
		stream, err := js.CreateStream(t.Context(), jetstream.StreamConfig{
			Name: "CLEANUP", Subjects: []string{"cleanup"}, Storage: jetstream.FileStorage,
		})
		require.NoError(t, err)
		consumer, err := stream.CreateConsumer(t.Context(), jetstream.ConsumerConfig{
			Durable: "cleanup", AckPolicy: jetstream.AckExplicitPolicy,
		})
		require.NoError(t, err)
		_, err = js.Publish(t.Context(), "cleanup", []byte("state"))
		require.NoError(t, err)
		msg, err := consumer.Next()
		require.NoError(t, err)
		require.NoError(t, msg.DoubleAck(t.Context()))
	})
	require.NoDirExists(t, dir)
}
