// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/messaging/broker/providers/nats"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

// A handler that only returns on a canceled context holds Unsubscribe for the
// whole drain timeout, so the configured value is observable: with the
// provider default (30s) Unsubscribe would not return within the bound below.
func TestBuilder_AppliesTheSubscriberDrainTimeoutFromConfig(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, js := natstest.ConnectJetStream(t, ns)

	const topic = "drain.created"
	_, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: "DRAIN", Subjects: []string{"drain.>"}})
	require.NoError(t, err)

	cfg := &brokerconfig.Config{
		Provider: brokerconfig.ProviderNATS,
		NATS:     &natsconfig.Config{SubscriberDrainTimeout: 50 * time.Millisecond},
	}
	built, err := factory.Builder(conn)(cfg, env())
	require.NoError(t, err)
	provider, ok := built.(*natsprovider.NATS)
	require.True(t, ok)

	started := make(chan struct{}, 1)
	handler := recoveryHandler{topic: topic, handle: func(ctx context.Context, _ *msg.Message) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
	}}
	sub := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
		Durable: "drain", FilterSubject: topic, AckPolicy: jetstream.AckExplicitPolicy, MaxDeliver: 1,
	})(provider)
	require.NoError(t, sub.Subscribe(t.Context(), handler))
	require.NoError(t, provider.Publish(t.Context(), *msg.NewMessage(topic, []byte("x"))))

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "message was never delivered")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		sub.Unsubscribe()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "Unsubscribe waited past the configured drain timeout")
	}
}

func TestDefaultConfig_MirrorsTheProviderDrainTimeout(t *testing.T) {
	t.Parallel()

	require.Equal(t, natsprovider.DefaultDrainTimeout, natsconfig.Default().SubscriberDrainTimeout)
}
