// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
)

// TestBuild_RecordsPublishMetricsOnTheMeter pins that the broker Build
// assembles publishes its metrics on the builder's meter, and that a builder
// without one still builds a working broker.
func TestBuild_RecordsPublishMetricsOnTheMeter(t *testing.T) {
	t.Parallel()

	const (
		metric  = "test_broker_messages_published_total"
		subject = "orders.created"
	)

	t.Run("meter records publishes", func(t *testing.T) {
		t.Parallel()

		col := metricstest.NewTestCollector()
		b, err := factory.New(new(brokerconfig.Default())).UseMeter(col).Build(stubProvider{})
		require.NoError(t, err)

		require.NoError(t, b.Publish(t.Context(), *msg.NewMessage(subject, []byte("hi"))))
		require.InDelta(t, 1, metricstest.GetCounterValue(t, col, metric, "subject", subject), 0)
	})

	t.Run("no meter publishes without metrics", func(t *testing.T) {
		t.Parallel()

		b, err := factory.New(new(brokerconfig.Default())).Build(stubProvider{})
		require.NoError(t, err)
		require.NoError(t, b.Publish(t.Context(), *msg.NewMessage(subject, []byte("hi"))))
	})
}

// TestCreateProvider_HandsTheMeterToTheProvider pins that the provider
// function receives the builder's meter, and nil when none is set.
func TestCreateProvider_HandsTheMeterToTheProvider(t *testing.T) {
	t.Parallel()

	col := metricstest.NewTestCollector()
	tests := []struct {
		name  string
		meter metrics.Meter
	}{
		{name: "meter set", meter: col},
		{name: "no meter"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got factory.ProviderEnv
			capture := func(_ *brokerconfig.Config, env factory.ProviderEnv) (broker.Provider, error) {
				got = env
				return stubProvider{}, nil
			}

			b := factory.New(new(brokerconfig.Default())).UseProvider(brokerconfig.ProviderNATS, capture)
			if tc.meter != nil {
				b.UseMeter(tc.meter)
			}
			_, err := b.CreateProvider()
			require.NoError(t, err)
			require.Equal(t, tc.meter, got.Meter)
		})
	}
}
