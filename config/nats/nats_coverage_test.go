// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func validConsumer() *natsconfig.Consumer {
	return &natsconfig.Consumer{
		DeliverPolicy: natsconfig.DeliverPolicyAll,
		AckPolicy:     natsconfig.AckPolicyExplicit,
		AckWait:       30 * time.Second,
		MaxDeliver:    -1,
		ReplayPolicy:  natsconfig.ReplayPolicyInstant,
		MaxAckPending: 1000,
		MaxWaiting:    512,
	}
}

func fieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func TestDefault(t *testing.T) {
	t.Parallel()

	c := natsconfig.Default()
	require.NotEmpty(t, c.Hosts)
	require.False(t, c.HasConsumers())
	require.False(t, c.UseConnectionURI())
	require.False(t, c.RecoveryEnabled())
	require.NoError(t, c.Validate())
}

func TestConfig_Consumers(t *testing.T) {
	t.Parallel()

	var nilCfg *natsconfig.Config
	_, ok := nilCfg.GetConsumerByName("x")
	require.False(t, ok)

	c := natsconfig.Default()
	_, ok = c.GetConsumerByName("x")
	require.False(t, ok)

	cons := validConsumer()
	c.Consumers = natsconfig.Consumers{"orders": cons}
	require.True(t, c.HasConsumers())
	got, ok := c.GetConsumerByName("orders")
	require.True(t, ok)
	require.Same(t, cons, got)
	_, ok = c.GetConsumerByName("missing")
	require.False(t, ok)
	require.NoError(t, c.Validate())

	cons.MaxWaiting = 0
	require.Contains(t, fieldErrors(t, c.Validate()), "Consumers")
}

func TestConfig_RecoveryEnabled(t *testing.T) {
	t.Parallel()

	c := natsconfig.Default()
	r := natsconfig.DefaultRecovery()
	c.Recovery = &r
	require.True(t, c.RecoveryEnabled())
	require.NoError(t, c.Validate())

	r.Enabled = false
	require.False(t, c.RecoveryEnabled())
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mut   func(*natsconfig.Config)
		field string
	}{
		{"no hosts", func(c *natsconfig.Config) { c.Hosts = nil }, "Hosts"},
		{"zero reconnect wait", func(c *natsconfig.Config) { c.ReconnectWait = 0 }, "ReconnectWait"},
		{"zero ping interval", func(c *natsconfig.Config) { c.PingInterval = 0 }, "PingInterval"},
		{"negative max reconnect", func(c *natsconfig.Config) { c.MaxReconnect = -1 }, "MaxReconnect"},
		{"zero max pings out", func(c *natsconfig.Config) { c.MaxPingsOut = 0 }, "MaxPingsOut"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := natsconfig.Default()
			tc.mut(&c)
			require.Contains(t, fieldErrors(t, c.Validate()), tc.field)
		})
	}
}

func TestConfig_ConnectionURI(t *testing.T) {
	t.Parallel()

	c := natsconfig.Default()
	c.Hosts = nil
	c.ConnectionURI = "nats://localhost:4222"
	require.True(t, c.UseConnectionURI())
	require.NoError(t, c.Validate())

	c.Username = "u"
	c.Password = "p"
	c.Token = "t"
	c.NkeySeed = "s"
	err := c.Validate()
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	for _, want := range []string{"username", "password", "token", "nkeySeed"} {
		require.ErrorContains(t, err, want+" must not be set")
	}
}

func TestRecovery_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, natsconfig.DefaultRecovery().Validate())
	require.NoError(t, natsconfig.Recovery{}.Validate(), "disabled recovery is not validated")

	r := natsconfig.DefaultRecovery()
	r.MaxRecoveryAttempts = 101
	r.HealthCheckSchedule = ""
	verrs := fieldErrors(t, r.Validate())
	require.Contains(t, verrs, "MaxRecoveryAttempts")
	require.Contains(t, verrs, "HealthCheckSchedule")
}

func TestConsumer_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mut   func(*natsconfig.Consumer)
		field string
	}{
		{"unknown deliver policy", func(c *natsconfig.Consumer) { c.DeliverPolicy = "first" }, "DeliverPolicy"},
		{"start sequence missing", func(c *natsconfig.Consumer) {
			c.DeliverPolicy = natsconfig.DeliverPolicyByStartSequence
		}, "OptStartSeq"},
		{"start time missing", func(c *natsconfig.Consumer) {
			c.DeliverPolicy = natsconfig.DeliverPolicyByStartTime
		}, "OptStartTime"},
		{"unknown ack policy", func(c *natsconfig.Consumer) { c.AckPolicy = "some" }, "AckPolicy"},
		{"zero ack wait", func(c *natsconfig.Consumer) { c.AckWait = 0 }, "AckWait"},
		{"max deliver below -1", func(c *natsconfig.Consumer) { c.MaxDeliver = -2 }, "MaxDeliver"},
		{"non-positive backoff", func(c *natsconfig.Consumer) {
			c.BackOff = []time.Duration{time.Second, 0}
		}, "BackOff"},
		{"unknown replay policy", func(c *natsconfig.Consumer) { c.ReplayPolicy = "fast" }, "ReplayPolicy"},
		{"zero max ack pending", func(c *natsconfig.Consumer) { c.MaxAckPending = 0 }, "MaxAckPending"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := validConsumer()
			tc.mut(c)
			require.Contains(t, fieldErrors(t, c.Validate()), tc.field)
		})
	}

	t.Run("valid variants", func(t *testing.T) {
		t.Parallel()
		seq := validConsumer()
		seq.DeliverPolicy = natsconfig.DeliverPolicyByStartSequence
		seq.OptStartSeq = 10
		require.NoError(t, seq.Validate())

		at := validConsumer()
		at.DeliverPolicy = natsconfig.DeliverPolicyByStartTime
		at.OptStartTime = new(time.Now())
		require.NoError(t, at.Validate())

		noAck := validConsumer()
		noAck.AckPolicy = natsconfig.AckPolicyNone
		noAck.AckWait = 0
		noAck.BackOff = []time.Duration{time.Second, 5 * time.Second}
		require.NoError(t, noAck.Validate())
	})
}

// TestConfig_NilConsumerEntry pins that an empty YAML entry
// (consumers: {orders: }) is reported as a validation error instead of
// panicking on a nil *Consumer.
func TestConfig_NilConsumerEntry(t *testing.T) {
	t.Parallel()

	c := natsconfig.Default()
	c.Consumers = natsconfig.Consumers{"orders": nil}
	require.NotPanics(t, func() {
		require.Contains(t, fieldErrors(t, c.Validate()), "Consumers")
	})
}
