// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"testing"
	"time"

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

	b := New(&config.Redis{}).
		UseLogger(logger).
		UseHealthCoordinator(coord)
	if b == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_NilOptions(t *testing.T) {
	b := New(nil).
		UseLogger(nil).
		UseHealthCoordinator(nil)
	if b == nil {
		t.Fatal("New() returned nil")
	}
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
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				if opts.PoolTimeout != DefaultPoolTimeout {
					t.Errorf("PoolTimeout = %v, want %v", opts.PoolTimeout, DefaultPoolTimeout)
				}
				if opts.MaxRetries != DefaultMaxRetries {
					t.Errorf("MaxRetries = %d, want %d", opts.MaxRetries, DefaultMaxRetries)
				}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(opts.Addrs) != 1 || opts.Addrs[0] != "redis-host:6380" {
		t.Errorf("Addrs = %v, want [redis-host:6380]", opts.Addrs)
	}
	if opts.Username != "myuser" {
		t.Errorf("Username = %q, want %q", opts.Username, "myuser")
	}
	if opts.Password != "mypass" {
		t.Errorf("Password = %q, want %q", opts.Password, "mypass")
	}
	if opts.DB != 3 {
		t.Errorf("DB = %d, want 3", opts.DB)
	}
	// Pool fields should be overlaid from config
	if opts.PoolSize != 50 {
		t.Errorf("PoolSize = %d, want 50", opts.PoolSize)
	}
	if opts.MinIdleConns != 10 {
		t.Errorf("MinIdleConns = %d, want 10", opts.MinIdleConns)
	}
	if opts.PoolTimeout != DefaultPoolTimeout {
		t.Errorf("PoolTimeout = %v, want %v", opts.PoolTimeout, DefaultPoolTimeout)
	}
	if opts.MaxRetries != DefaultMaxRetries {
		t.Errorf("MaxRetries = %d, want %d", opts.MaxRetries, DefaultMaxRetries)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.TLSConfig == nil {
		t.Error("expected TLSConfig to be set for rediss:// URI")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.MasterName != "mymaster" {
		t.Errorf("MasterName = %q, want %q", opts.MasterName, "mymaster")
	}
	if opts.SentinelPassword != "sentpass" {
		t.Errorf("SentinelPassword = %q, want %q", opts.SentinelPassword, "sentpass")
	}
}

func TestUniversalOptions_ConnectionURI_InvalidURI(t *testing.T) {
	b := New(&config.Redis{
		ConnectionURI: "not-a-valid-uri",
	})

	_, err := b.UniversalOptions()
	if err == nil {
		t.Fatal("expected error for invalid URI")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(opts.Addrs) != 2 {
		t.Errorf("Addrs len = %d", len(opts.Addrs))
	}
	if opts.Password != "pass" {
		t.Errorf("Password = %q", opts.Password)
	}
	if opts.Username != "user" {
		t.Errorf("Username = %q", opts.Username)
	}
	if opts.DB != 2 {
		t.Errorf("DB = %d", opts.DB)
	}
	if opts.PoolSize != 20 {
		t.Errorf("PoolSize = %d", opts.PoolSize)
	}
	if !opts.ReadOnly {
		t.Error("ReadOnly should be true")
	}
	if !opts.RouteByLatency {
		t.Error("RouteByLatency should be true")
	}
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
			if got := b.detectMode(); got != tt.want {
				t.Errorf("detectMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultConstants(t *testing.T) {
	if DefaultPoolTimeout != 10*time.Second {
		t.Errorf("DefaultPoolTimeout = %v", DefaultPoolTimeout)
	}
	if DefaultMaxRetries != 5 {
		t.Errorf("DefaultMaxRetries = %d", DefaultMaxRetries)
	}
	if DefaultPingTimeout != 5*time.Second {
		t.Errorf("DefaultPingTimeout = %v", DefaultPingTimeout)
	}
}
