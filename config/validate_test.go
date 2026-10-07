// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	redisconfig "github.com/altessa-s/go-atlas/config/redis"
)

func TestValidate_Auth_NilSubConfigs(t *testing.T) {
	cfg := authconfig.Config{}
	// Auth with nil sub-configs should still pass (NilOrNotEmpty)
	require.NoError(t, cfg.Validate())
}

func TestValidate_Grpc_Valid(t *testing.T) {
	cfg := grpcconfig.Default()
	require.NoError(t, cfg.Validate())
}

func TestValidate_Grpc_InvalidAddress(t *testing.T) {
	cfg := grpcconfig.Default()
	cfg.ListenAddress = "not-valid"
	require.Error(t, cfg.Validate())
}

func TestValidate_Http_Valid(t *testing.T) {
	cfg := httpconfig.Default()
	require.NoError(t, cfg.Validate())
}

func TestValidate_Http_InvalidAddress(t *testing.T) {
	cfg := httpconfig.Default()
	cfg.ListenAddress = "invalid"
	require.Error(t, cfg.Validate())
}

func TestValidate_Mongodb_Valid(t *testing.T) {
	cfg := mongoconfig.Default()
	cfg.Database = "testdb"
	require.NoError(t, cfg.Validate())
}

func TestValidate_Mongodb_NoHosts(t *testing.T) {
	cfg := mongoconfig.Default()
	cfg.Database = "testdb"
	cfg.Hosts = nil
	require.Error(t, cfg.Validate())
}

func TestValidate_Mongodb_NoDatabase(t *testing.T) {
	cfg := mongoconfig.Default()
	require.Error(t, cfg.Validate())
}

func TestValidate_Mongodb_DirectConnectionHosts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		hosts            []string
		directConnection bool
		wantErr          bool
	}{
		{name: "multiple hosts without direct connection", hosts: []string{"host1:27017", "host2:27017"}},
		{name: "SRV host without direct connection", hosts: []string{"mongodb+srv://cluster.example.com"}},
		{name: "single host with direct connection", hosts: []string{"host1:27017"}, directConnection: true},
		{
			name:             "multiple hosts with direct connection",
			hosts:            []string{"host1:27017", "host2:27017"},
			directConnection: true,
			wantErr:          true,
		},
		{
			name:             "SRV host with direct connection",
			hosts:            []string{"mongodb+srv://cluster.example.com"},
			directConnection: true,
			wantErr:          true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := mongoconfig.Default()
			cfg.Database = "testdb"
			cfg.Hosts = tc.hosts
			cfg.DirectConnection = tc.directConnection

			if tc.wantErr {
				require.Error(t, cfg.Validate())
				return
			}
			require.NoError(t, cfg.Validate())
		})
	}
}

func TestValidate_Redis_Valid(t *testing.T) {
	cfg := redisconfig.Default()
	require.NoError(t, cfg.Validate())
}

func TestValidate_Redis_NoHosts(t *testing.T) {
	cfg := redisconfig.Default()
	cfg.Hosts = nil
	require.Error(t, cfg.Validate())
}

func TestValidate_Nats_Valid(t *testing.T) {
	cfg := natsconfig.Default()
	require.NoError(t, cfg.Validate())
}

func TestValidate_Nats_NoHosts(t *testing.T) {
	cfg := natsconfig.Default()
	cfg.Hosts = nil
	require.Error(t, cfg.Validate())
}

func TestValidate_Nats_ZeroPingInterval(t *testing.T) {
	cfg := natsconfig.Default()
	cfg.PingInterval = 0 * time.Second
	require.Error(t, cfg.Validate())
}

// --- ConnectionURI validation tests ---

func TestValidate_Nats_ConnectionURI_Only(t *testing.T) {
	cfg := natsconfig.Default()
	cfg.ConnectionURI = "nats://user:pass@nats:4222"
	require.NoError(t, cfg.Validate())
}

func TestValidate_Nats_ConnectionURI_NoHosts(t *testing.T) {
	cfg := natsconfig.Config{
		ConnectionURI:  "nats://nats:4222",
		PingInterval:   10 * time.Second,
		ReconnectWait:  10 * time.Second,
		ConnectTimeout: 5 * time.Second,
		MaxPingsOut:    3,
	}
	require.NoError(t, cfg.Validate())
}

func TestValidate_Nats_ConnectionURI_ConflictUsername(t *testing.T) {
	cfg := natsconfig.Default()
	cfg.ConnectionURI = "nats://nats:4222"
	cfg.Username = "user"
	require.Error(t, cfg.Validate())
}

func TestValidate_Nats_ConnectionURI_ConflictPassword(t *testing.T) {
	cfg := natsconfig.Default()
	cfg.ConnectionURI = "nats://nats:4222"
	cfg.Password = "pass"
	require.Error(t, cfg.Validate())
}

func TestValidate_Nats_ConnectionURI_ConflictToken(t *testing.T) {
	cfg := natsconfig.Default()
	cfg.ConnectionURI = "nats://nats:4222"
	cfg.Token = "tok"
	require.Error(t, cfg.Validate())
}

func TestValidate_Nats_ConnectionURI_ConflictNkeySeed(t *testing.T) {
	cfg := natsconfig.Default()
	cfg.ConnectionURI = "nats://nats:4222"
	cfg.NkeySeed = "SUACSSL3UAHUDXKFSNVUZRF5UHPMWZ6BFDTJ7M6USDXIEDNPPQYYYCU3VY"
	require.Error(t, cfg.Validate())
}

func TestValidate_Nats_ConnectionURI_MultipleConflicts(t *testing.T) {
	cfg := natsconfig.Default()
	cfg.ConnectionURI = "nats://nats:4222"
	cfg.Username = "user"
	cfg.Token = "tok"
	require.Error(t, cfg.Validate())
}

func TestValidate_Redis_ConnectionURI_Only(t *testing.T) {
	cfg := redisconfig.Default()
	cfg.ConnectionURI = "redis://localhost:6379/0"
	require.NoError(t, cfg.Validate())
}

func TestValidate_Redis_ConnectionURI_NoHosts(t *testing.T) {
	cfg := redisconfig.Config{
		ConnectionURI:  "redis://localhost:6379/0",
		ConnectTimeout: 5 * time.Second,
		SocketTimeout:  5 * time.Second,
		IdleTimeout:    5 * time.Second,
	}
	require.NoError(t, cfg.Validate())
}

func TestValidate_Redis_ConnectionURI_ConflictUsername(t *testing.T) {
	cfg := redisconfig.Default()
	cfg.ConnectionURI = "redis://localhost:6379/0"
	cfg.Username = "user"
	require.Error(t, cfg.Validate())
}

func TestValidate_Redis_ConnectionURI_ConflictPassword(t *testing.T) {
	cfg := redisconfig.Default()
	cfg.ConnectionURI = "redis://localhost:6379/0"
	cfg.Password = "pass"
	require.Error(t, cfg.Validate())
}

func TestValidate_Mongodb_ConnectionURI_Only(t *testing.T) {
	cfg := mongoconfig.Default()
	cfg.ConnectionURI = "mongodb://localhost:27017/testdb"
	cfg.Database = "testdb"
	require.NoError(t, cfg.Validate())
}

func TestValidate_Mongodb_ConnectionURI_NoHosts(t *testing.T) {
	cfg := mongoconfig.Config{
		ConnectionURI:  "mongodb://localhost:27017/testdb",
		Database:       "testdb",
		ConnectTimeout: 30 * time.Second,
		MaxPoolSize:    100,
	}
	require.NoError(t, cfg.Validate())
}

func TestValidate_Mongodb_ConnectionURI_DatabaseRequired(t *testing.T) {
	cfg := mongoconfig.Default()
	cfg.ConnectionURI = "mongodb://localhost:27017/testdb"
	// Database is still required even with URI
	require.Error(t, cfg.Validate())
}

func TestValidate_Mongodb_ConnectionURI_ConflictCredentials(t *testing.T) {
	cfg := mongoconfig.Default()
	cfg.ConnectionURI = "mongodb://user:pass@localhost:27017/testdb"
	cfg.Database = "testdb"
	cfg.Credentials = &mongoconfig.Credentials{
		AuthMechanism: mongoconfig.AuthMechanismTypePLAIN,
		Plain:         &mongoconfig.PLAINCredentials{Username: "user", Password: "pass"},
	}
	require.Error(t, cfg.Validate())
}

func TestValidate_Nats_UseConnectionURI(t *testing.T) {
	n := &natsconfig.Config{}
	require.False(t, n.UseConnectionURI())
	n.ConnectionURI = "nats://localhost:4222"
	require.True(t, n.UseConnectionURI())
}

func TestValidate_Redis_UseConnectionURI(t *testing.T) {
	r := &redisconfig.Config{}
	require.False(t, r.UseConnectionURI())
	r.ConnectionURI = "redis://localhost:6379"
	require.True(t, r.UseConnectionURI())
}

func TestValidate_Mongodb_UseConnectionURI(t *testing.T) {
	m := &mongoconfig.Config{}
	require.False(t, m.UseConnectionURI())
	m.ConnectionURI = "mongodb://localhost:27017"
	require.True(t, m.UseConnectionURI())
}
