// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nkeys"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"
)

func TestNew_Default(t *testing.T) {
	b := New(nil)
	if b == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithOptions(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	coord := health.New()
	defer coord.Close()

	b := New(&config.Nats{}).
		UseLogger(logger).
		UseHealthCoordinator(coord)
	if b == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNatsOptions(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Nats
		wantErr bool
	}{
		{
			"valid basic",
			&config.Nats{
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
			&config.Nats{
				Hosts: []string{"nats://localhost:4222"},
				Token: "mytoken",
			},
			false,
		},
		{
			"with user/pass auth",
			&config.Nats{
				Hosts:    []string{"nats://localhost:4222"},
				Username: "user",
				Password: "pass",
			},
			false,
		},
		{
			"with nkey auth",
			func() *config.Nats {
				kp, _ := nkeys.CreateUser()
				seed, _ := kp.Seed()
				return &config.Nats{
					Hosts:    []string{"nats://localhost:4222"},
					NkeySeed: config.Secret(seed),
				}
			}(),
			false,
		},
		{
			"with invalid nkey seed",
			&config.Nats{
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
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && len(opts) == 0 {
				t.Error("expected non-empty options")
			}
		})
	}
}

func TestNatsOptions_ConnectionURI(t *testing.T) {
	b := New(&config.Nats{
		ConnectionURI:  "nats://user:pass@nats:4222",
		ConnectTimeout: 5 * time.Second,
		ReconnectWait:  2 * time.Second,
		PingInterval:   time.Minute,
		MaxPingsOut:    3,
	})
	opts, err := b.NatsOptions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected non-empty options")
	}
}

func TestNatsOptions_ConnectionURI_SkipsAuth(t *testing.T) {
	// When URI is set, auth options should not be added even if
	// cfg has zero-value auth fields (they should be empty).
	b := New(&config.Nats{
		ConnectionURI:  "nats://nats:4222",
		ConnectTimeout: 5 * time.Second,
		ReconnectWait:  2 * time.Second,
		PingInterval:   time.Minute,
		MaxPingsOut:    3,
	})
	opts, err := b.NatsOptions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected non-empty options")
	}
}

func TestNatsOptions_MaxReconnect(t *testing.T) {
	b := New(&config.Nats{
		Hosts:        []string{"nats://localhost:4222"},
		MaxReconnect: 0, // should default to UnlimitedReconnects
	})
	_, err := b.NatsOptions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConsumerConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.NatsConsumer
		wantErr bool
	}{
		{
			"valid",
			&config.NatsConsumer{
				Description:    "test consumer",
				DurableName:    "test-durable",
				FilterSubjects: []string{"events.>"},
				MaxAckPending:  100,
				AckWait:        30 * time.Second,
				MaxDeliver:     5,
				DeliverPolicy:  config.DeliverPolicyAll,
				AckPolicy:      config.AckPolicyExplicit,
			},
			false,
		},
		{
			"by start sequence",
			&config.NatsConsumer{
				DurableName:   "seq-consumer",
				DeliverPolicy: config.DeliverPolicyByStartSequence,
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
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && consumerCfg == nil {
				t.Error("expected non-nil consumer config")
			}
		})
	}
}

func TestConstants(t *testing.T) {
	if UnlimitedReconnects != -1 {
		t.Errorf("UnlimitedReconnects = %d", UnlimitedReconnects)
	}
	if UnlimitedReconnectBuffer != -1 {
		t.Errorf("UnlimitedReconnectBuffer = %d", UnlimitedReconnectBuffer)
	}
}
