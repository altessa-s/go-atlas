// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider_test

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

// A publish on a closed connection reports the provider-neutral
// broker.ErrConnectionClosed while still matching the nats.go sentinel.
func TestPublish_ClosedConnectionMatchesBrokerSentinel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		publish func(*natsprovider.NATS) error
		// wantDriverErr: the async batch path formats the driver error
		// without %w, so only the synchronous path keeps nats' sentinel.
		wantDriverErr bool
	}{
		{name: "publish", wantDriverErr: true, publish: func(p *natsprovider.NATS) error {
			return p.Publish(t.Context(), *msg.NewMessage("orders.created", []byte("a")))
		}},
		{name: "publish_batch", publish: func(p *natsprovider.NATS) error {
			return p.PublishBatch(t.Context(),
				*msg.NewMessage("orders.created", []byte("a")),
				*msg.NewMessage("orders.created", []byte("b")))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ns := natstest.StartNATSServer(t)
			conn, _ := natstest.ConnectJetStream(t, ns)
			provider, err := natsprovider.New(conn)
			require.NoError(t, err)
			conn.Close()

			err = tc.publish(provider)
			require.ErrorIs(t, err, broker.ErrConnectionClosed)
			if tc.wantDriverErr {
				require.ErrorIs(t, err, nats.ErrConnectionClosed)
			}
		})
	}
}

// Other publish failures are not classified as a closed connection.
func TestPublish_OtherErrorIsNotConnectionClosed(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, _ := natstest.ConnectJetStream(t, ns)
	provider, err := natsprovider.New(conn, natsprovider.WithAllowedSubjects("orders.>"))
	require.NoError(t, err)

	err = provider.Publish(t.Context(), *msg.NewMessage("billing.paid", []byte("a")))
	require.Error(t, err)
	require.False(t, errors.Is(err, broker.ErrConnectionClosed))
}
