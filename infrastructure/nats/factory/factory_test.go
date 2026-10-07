// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/redacted"
	"github.com/altessa-s/go-atlas/observability/health"

	natsconfig "github.com/altessa-s/go-atlas/config/nats"
)

func TestNew_Default(t *testing.T) {
	b := New(nil)
	require.NotNil(t, b)
}

func TestNew_WithOptions(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	coord := health.New()
	defer coord.Close()

	b := New(&natsconfig.Config{}).
		UseLogger(logger).
		UseHealthCoordinator(coord)
	require.NotNil(t, b)
}

func TestNatsOptions(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *natsconfig.Config
		wantErr bool
	}{
		{
			"valid basic",
			&natsconfig.Config{
				Hosts:          []string{"nats://localhost:4222"},
				ClientName:     "test-client",
				ConnectTimeout: 5 * time.Second,
				ReconnectWait:  2 * time.Second,
				PingInterval:   time.Minute,
				MaxPingsOut:    3,
			},
			false,
		},
		{
			"with token auth",
			&natsconfig.Config{
				Hosts: []string{"nats://localhost:4222"},
				Token: "mytoken",
			},
			false,
		},
		{
			"with user/pass auth",
			&natsconfig.Config{
				Hosts:    []string{"nats://localhost:4222"},
				Username: "user",
				Password: "pass",
			},
			false,
		},
		{
			"with nkey auth",
			func() *natsconfig.Config {
				kp, _ := nkeys.CreateUser()
				seed, _ := kp.Seed()
				return &natsconfig.Config{
					Hosts:    []string{"nats://localhost:4222"},
					NkeySeed: redacted.RedactedString(seed),
				}
			}(),
			false,
		},
		{
			"with invalid nkey seed",
			&natsconfig.Config{
				Hosts:    []string{"nats://localhost:4222"},
				NkeySeed: "invalid-seed",
			},
			true,
		},
		{
			"nil config",
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New(tt.cfg)
			opts, err := b.NatsOptions()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, opts)
			}
		})
	}
}

func TestNatsOptions_ConnectionURI(t *testing.T) {
	b := New(&natsconfig.Config{
		ConnectionURI:  "nats://user:pass@nats:4222",
		ConnectTimeout: 5 * time.Second,
		ReconnectWait:  2 * time.Second,
		PingInterval:   time.Minute,
		MaxPingsOut:    3,
	})
	opts, err := b.NatsOptions()
	require.NoError(t, err)
	require.NotEmpty(t, opts)
}

func TestNatsOptions_ConnectionURI_SkipsAuth(t *testing.T) {
	// When URI is set, auth options should not be added even if
	// cfg has zero-value auth fields (they should be empty).
	b := New(&natsconfig.Config{
		ConnectionURI:  "nats://nats:4222",
		ConnectTimeout: 5 * time.Second,
		ReconnectWait:  2 * time.Second,
		PingInterval:   time.Minute,
		MaxPingsOut:    3,
	})
	opts, err := b.NatsOptions()
	require.NoError(t, err)
	require.NotEmpty(t, opts)
}

func TestNatsOptions_MaxReconnect(t *testing.T) {
	b := New(&natsconfig.Config{
		Hosts:        []string{"nats://localhost:4222"},
		MaxReconnect: 0, // should default to UnlimitedReconnects
	})
	_, err := b.NatsOptions()
	require.NoError(t, err)
}

func TestConsumerConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *natsconfig.Consumer
		wantErr bool
	}{
		{
			"valid",
			&natsconfig.Consumer{
				Description:    "test consumer",
				DurableName:    "test-durable",
				FilterSubjects: []string{"events.>"},
				MaxAckPending:  100,
				AckWait:        30 * time.Second,
				MaxDeliver:     5,
				DeliverPolicy:  natsconfig.DeliverPolicyAll,
				AckPolicy:      natsconfig.AckPolicyExplicit,
			},
			false,
		},
		{
			"by start sequence",
			&natsconfig.Consumer{
				DurableName:   "seq-consumer",
				DeliverPolicy: natsconfig.DeliverPolicyByStartSequence,
				OptStartSeq:   100,
			},
			false,
		},
		{
			"nil config",
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consumerCfg, err := ConsumerConfig(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, consumerCfg)
			}
		})
	}
}
