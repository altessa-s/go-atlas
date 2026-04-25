// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"
)

func TestNew_Default(t *testing.T) {
	b := New(nil)
	require.NotNil(t, b)
}

func TestNew_WithOptions(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	coord := health.New()
	defer coord.Close()

	b := New(&config.Redis{}).
		UseLogger(logger).
		UseHealthCoordinator(coord)
	require.NotNil(t, b)
}

func TestNew_NilOptions(t *testing.T) {
	b := New(nil).
		UseLogger(nil).
		UseHealthCoordinator(nil)
	require.NotNil(t, b)
}

func TestUniversalOptions(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Redis
		wantErr bool
	}{
		{
			"valid standalone",
			&config.Redis{
				Hosts:    []string{"localhost:6379"},
				Password: "pass",
				Database: 1,
				PoolSize: 10,
			},
			false,
		},
		{
			"valid sentinel",
			&config.Redis{
				Hosts:            []string{"sentinel1:26379"},
				MasterName:       "mymaster",
				SentinelPassword: "sentpass",
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
			b := New(tt.cfg)
			opts, err := b.UniversalOptions()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, DefaultPoolTimeout, opts.PoolTimeout)
				require.Equal(t, DefaultMaxRetries, opts.MaxRetries)
			}
		})
	}
}

func TestUniversalOptions_ConnectionURI(t *testing.T) {
	b := New(&config.Redis{
		ConnectionURI:      "redis://myuser:mypass@redis-host:6380/3",
		PoolSize:           50,
		MinIdleConnections: 10,
		ConnectTimeout:     3 * time.Second,
		SocketTimeout:      5 * time.Second,
		IdleTimeout:        60 * time.Second,
	})

	opts, err := b.UniversalOptions()
	require.NoError(t, err)

	require.Equal(t, []string{"redis-host:6380"}, opts.Addrs)
	require.Equal(t, "myuser", opts.Username)
	require.Equal(t, "mypass", opts.Password)
	require.Equal(t, 3, opts.DB)
	// Pool fields should be overlaid from config
	require.Equal(t, 50, opts.PoolSize)
	require.Equal(t, 10, opts.MinIdleConns)
	require.Equal(t, DefaultPoolTimeout, opts.PoolTimeout)
	require.Equal(t, DefaultMaxRetries, opts.MaxRetries)
}

func TestUniversalOptions_ConnectionURI_TLS(t *testing.T) {
	b := New(&config.Redis{
		ConnectionURI:  "rediss://redis-host:6380/0",
		PoolSize:       10,
		ConnectTimeout: 3 * time.Second,
		SocketTimeout:  5 * time.Second,
		IdleTimeout:    60 * time.Second,
	})

	opts, err := b.UniversalOptions()
	require.NoError(t, err)
	require.NotNil(t, opts.TLSConfig, "expected TLSConfig to be set for rediss:// URI")
}

func TestUniversalOptions_ConnectionURI_Sentinel(t *testing.T) {
	b := New(&config.Redis{
		ConnectionURI:    "redis://redis-host:6380/0",
		MasterName:       "mymaster",
		SentinelPassword: "sentpass",
		ConnectTimeout:   3 * time.Second,
		SocketTimeout:    5 * time.Second,
		IdleTimeout:      60 * time.Second,
	})

	opts, err := b.UniversalOptions()
	require.NoError(t, err)
	require.Equal(t, "mymaster", opts.MasterName)
	require.Equal(t, "sentpass", opts.SentinelPassword)
}

func TestUniversalOptions_ConnectionURI_InvalidURI(t *testing.T) {
	b := New(&config.Redis{
		ConnectionURI: "not-a-valid-uri",
	})

	_, err := b.UniversalOptions()
	require.Error(t, err)
}

func TestUniversalOptions_Fields(t *testing.T) {
	b := New(&config.Redis{
		Hosts:              []string{"host1:6379", "host2:6379"},
		Password:           "pass",
		Username:           "user",
		Database:           2,
		PoolSize:           20,
		MinIdleConnections: 5,
		ConnectTimeout:     3 * time.Second,
		SocketTimeout:      5 * time.Second,
		IdleTimeout:        60 * time.Second,
		MaxConnectionAge:   120 * time.Second,
		MaxRedirects:       3,
		ReadOnly:           true,
		RouteByLatency:     true,
		RouteRandomly:      false,
	})

	opts, err := b.UniversalOptions()
	require.NoError(t, err)

	require.Len(t, opts.Addrs, 2)
	require.Equal(t, "pass", opts.Password)
	require.Equal(t, "user", opts.Username)
	require.Equal(t, 2, opts.DB)
	require.Equal(t, 20, opts.PoolSize)
	require.True(t, opts.ReadOnly, "ReadOnly should be true")
	require.True(t, opts.RouteByLatency, "RouteByLatency should be true")
}

func TestDetectMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Redis
		want string
	}{
		{"standalone", &config.Redis{Hosts: []string{"localhost:6379"}}, "standalone"},
		{"cluster", &config.Redis{Hosts: []string{"h1:6379", "h2:6379"}}, "cluster"},
		{"sentinel", &config.Redis{Hosts: []string{"s1:26379"}, MasterName: "master"}, "sentinel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New(tt.cfg)
			require.Equal(t, tt.want, b.detectMode())
		})
	}
}
