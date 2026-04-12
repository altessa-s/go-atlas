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

	b := New(&config.Mongodb{}).
		UseLogger(logger).
		UseHealthCoordinator(coord)
	require.NotNil(t, b)
}

func TestClientOptions(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Mongodb
		wantErr bool
	}{
		{
			"valid basic",
			&config.Mongodb{
				Hosts:            []string{"localhost:27017"},
				Database:         "testdb",
				DirectConnection: true,
				MaxPoolSize:      100,
				MinPoolSize:      10,
				ConnectTimeout:   10 * time.Second,
				MaxIdleTimeout:   60 * time.Second,
				RetryReads:       true,
				RetryWrites:      true,
			},
			false,
		},
		{
			"with replica set",
			&config.Mongodb{
				Hosts:      []string{"host1:27017", "host2:27017"},
				Database:   "testdb",
				ReplicaSet: "rs0",
			},
			false,
		},
		{
			"with compressors",
			&config.Mongodb{
				Hosts:                []string{"localhost:27017"},
				Database:             "testdb",
				Compressors:          config.MongoCompressionTypes{"snappy"},
				ZlibCompressionLevel: -1,
			},
			false,
		},
		{
			"with X509 auth",
			&config.Mongodb{
				Hosts:    []string{"localhost:27017"},
				Database: "testdb",
				Credentials: &config.MongodbCredentials{
					AuthMechanism: config.MongoAuthMechanismTypeX509,
				},
			},
			false,
		},
		{
			"with PLAIN auth",
			&config.Mongodb{
				Hosts:    []string{"localhost:27017"},
				Database: "testdb",
				Credentials: &config.MongodbCredentials{
					AuthMechanism: config.MongoAuthMechanismTypePLAIN,
					Plain: &config.MongoPLAINCredentials{
						Username: "user",
						Password: "pass",
					},
				},
			},
			false,
		},
		{
			"with SCRAM-SHA-256 auth",
			&config.Mongodb{
				Hosts:    []string{"localhost:27017"},
				Database: "testdb",
				Credentials: &config.MongodbCredentials{
					AuthMechanism: config.MongoAuthMechanismTypeSCRAMSHA256,
					Scram: &config.MongoSCRAMCredentials{
						Username:   "user",
						Password:   "pass",
						AuthSource: "admin",
					},
				},
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
			if tt.cfg == nil {
				_, err := b.Build(t.Context())
				require.Error(t, err)
				return
			}
			opts, err := b.ClientOptions()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, opts)
			}
		})
	}
}

func TestClientOptions_ConnectionURI(t *testing.T) {
	b := New(&config.Mongodb{
		ConnectionURI:  "mongodb://user:pass@mongo1:27017,mongo2:27017/testdb?replicaSet=rs0",
		Database:       "testdb",
		MaxPoolSize:    200,
		MinPoolSize:    10,
		ConnectTimeout: 15 * time.Second,
		MaxIdleTimeout: 30 * time.Second,
		RetryReads:     true,
		RetryWrites:    true,
	})
	opts, err := b.ClientOptions()
	require.NoError(t, err)
	require.NotNil(t, opts)
}

func TestClientOptions_ConnectionURI_PoolOverlay(t *testing.T) {
	b := New(&config.Mongodb{
		ConnectionURI:  "mongodb://localhost:27017/testdb",
		Database:       "testdb",
		MaxPoolSize:    50,
		MinPoolSize:    5,
		ConnectTimeout: 10 * time.Second,
		RetryReads:     false,
		RetryWrites:    false,
	})
	opts, err := b.ClientOptions()
	require.NoError(t, err)
	require.NotNil(t, opts)
}

func TestBuildCredential(t *testing.T) {
	tests := []struct {
		name     string
		creds    *config.MongodbCredentials
		wantMech string
	}{
		{
			"nil creds",
			nil,
			"",
		},
		{
			"X509",
			&config.MongodbCredentials{AuthMechanism: config.MongoAuthMechanismTypeX509},
			"MONGODB-X509",
		},
		{
			"PLAIN",
			&config.MongodbCredentials{
				AuthMechanism: config.MongoAuthMechanismTypePLAIN,
				Plain:         &config.MongoPLAINCredentials{Username: "u", Password: "p"},
			},
			"PLAIN",
		},
		{
			"PLAIN nil creds",
			&config.MongodbCredentials{AuthMechanism: config.MongoAuthMechanismTypePLAIN},
			"",
		},
		{
			"SCRAM-SHA-1",
			&config.MongodbCredentials{
				AuthMechanism: config.MongoAuthMechanismTypeSCRAMSHA1,
				Scram:         &config.MongoSCRAMCredentials{Username: "u", Password: "p", AuthSource: "admin"},
			},
			"SCRAM-SHA-1",
		},
		{
			"SCRAM nil creds",
			&config.MongodbCredentials{AuthMechanism: config.MongoAuthMechanismTypeSCRAMSHA1},
			"",
		},
		{
			"unknown mechanism",
			&config.MongodbCredentials{AuthMechanism: config.MongoAuthMechanismType("unknown")},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New(&config.Mongodb{Credentials: tt.creds})
			cred := b.buildCredential()
			require.Equal(t, tt.wantMech, cred.AuthMechanism)
		})
	}
}
