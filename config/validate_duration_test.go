// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	redisconfig "github.com/altessa-s/go-atlas/config/redis"
)

// TestValidate_NegativeDurationRejected sets one duration of an otherwise
// valid configuration to a negative value and asserts that Validate rejects
// it, both for durations that must be positive and for durations where zero
// means "unset" or "no limit".
func TestValidate_NegativeDurationRejected(t *testing.T) {
	t.Parallel()

	const neg = -time.Second
	tests := []struct {
		name     string
		validate func() error
	}{
		{"Redis.SocketTimeout (positive)", func() error {
			c := redisconfig.Default()
			c.SocketTimeout = neg
			return c.Validate()
		}},
		{"Redis.ConnectTimeout (zero allowed)", func() error {
			c := redisconfig.Default()
			c.ConnectTimeout = neg
			return c.Validate()
		}},
		{"Nats.ReconnectWait (positive)", func() error {
			c := natsconfig.Default()
			c.ReconnectWait = neg
			return c.Validate()
		}},
		{"Nats.ConnectTimeout (zero allowed)", func() error {
			c := natsconfig.Default()
			c.ConnectTimeout = neg
			return c.Validate()
		}},
		{"HTTP.ReadTimeout (zero allowed)", func() error {
			c := httpconfig.Default()
			c.ReadTimeout = neg
			return c.Validate()
		}},
		{"Mongo.ConnectTimeout (zero allowed)", func() error {
			c := mongoconfig.Default()
			c.Database = "app"
			c.ConnectTimeout = neg
			return c.Validate()
		}},
		{"Saga.ExecutionTimeout (zero means default)", func() error {
			c := validSaga()
			c.ExecutionTimeout = neg
			return c.Validate()
		}},
		{"GRPC.ConnectionTimeout (positive when set)", func() error {
			c := grpcconfig.Default()
			c.ConnectionTimeout = new(neg)
			return c.Validate()
		}},
		{"GRPC.KeepAlive.Time (zero allowed)", func() error {
			return (&grpcconfig.KeepAlive{Time: neg}).Validate()
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, tc.validate())
		})
	}
}
