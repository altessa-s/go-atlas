// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

// TestPublishBatch_CanceledContextStoresNothing verifies that a canceled
// context is observed before any message of the batch is submitted, so a
// returned cancellation error never hides a fully published batch.
func TestPublishBatch_CanceledContextStoresNothing(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, js := natstest.ConnectJetStream(t, ns)
	_, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)
	provider, err := natsprovider.New(conn)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = provider.PublishBatch(ctx,
		msg.Message{Topic: "orders.a", Data: []byte("a")},
		msg.Message{Topic: "orders.b", Data: []byte("b")},
	)
	require.ErrorIs(t, err, context.Canceled)

	stream, err := js.Stream(t.Context(), "ORDERS")
	require.NoError(t, err)
	info, err := stream.Info(t.Context())
	require.NoError(t, err)
	require.Zero(t, info.State.Msgs)
}
