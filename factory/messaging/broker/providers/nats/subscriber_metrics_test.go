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
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	brokerfactory "github.com/altessa-s/go-atlas/factory/messaging/broker"
	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

// TestBuilder_RecordsSubscriberMetricsOnTheMeter pins that the provider the
// factory builds records its subscriber metrics on the broker builder's meter,
// and that a builder without one still delivers messages.
func TestBuilder_RecordsSubscriberMetricsOnTheMeter(t *testing.T) {
	t.Parallel()

	const (
		metric = "test_broker_messages_received_total"
		topic  = "orders.created"
	)

	col := metricstest.NewTestCollector()
	tests := []struct {
		name  string
		meter metrics.Meter
	}{
		{name: "meter records deliveries", meter: col},
		{name: "no meter delivers without metrics"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ns := natstest.StartNATSServer(t)
			conn := natstest.ConnectNATS(t, ns)

			b := brokerfactory.New(&brokerconfig.Config{Provider: brokerconfig.ProviderNATS}).
				UseProvider(brokerconfig.ProviderNATS, factory.Builder(conn))
			if tc.meter != nil {
				b.UseMeter(tc.meter)
			}
			built, err := b.CreateProvider()
			require.NoError(t, err)
			provider, ok := built.(*natsprovider.NATS)
			require.True(t, ok)

			_, err = provider.JetStream().CreateStream(t.Context(), jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
			require.NoError(t, err)

			received := make(chan struct{}, 1)
			handler := recoveryHandler{topic: topic, handle: func(_ context.Context, m *msg.Message) {
				_ = m.Ack()
				received <- struct{}{}
			}}
			sub := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
				Durable: "proc", FilterSubject: topic, AckPolicy: jetstream.AckExplicitPolicy,
			})(provider)
			require.NoError(t, sub.Subscribe(t.Context(), handler))
			t.Cleanup(sub.Unsubscribe)

			require.NoError(t, provider.Publish(t.Context(), *msg.NewMessage(topic, []byte("hi"))))
			select {
			case <-received:
			case <-time.After(5 * time.Second):
				require.FailNow(t, "message was never delivered through the factory-built provider")
			}

			if tc.meter == nil {
				return
			}
			testhelpers.WaitFor(t, 5*time.Second, func() bool {
				return metricstest.GetCounterValue(t, col, metric, "subject", topic) == 1
			}, "the delivery was never counted on the builder's meter")
		})
	}
}
