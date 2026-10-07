// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/converter"
	"github.com/altessa-s/go-atlas/observability/health"

	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
)

func TestNew_Default(t *testing.T) {
	b := New(nil)
	require.NotNil(t, b)
}

func TestNew_WithOptions(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	coord := health.New()
	defer coord.Close()

	b := New(&mongoconfig.Config{}).
		UseLogger(logger).
		UseHealthCoordinator(coord)
	require.NotNil(t, b)
}

func TestUseConverterOptions_AccumulatesAcrossCalls(t *testing.T) {
	b := New(&mongoconfig.Config{Database: "x"}).
		UseConverterOptions(converter.WithIgnoreZeroValues()).
		UseConverterOptions(converter.WithIgnoreNilValues())
	require.Len(t, b.converterOpts, 2)
}

func TestUseConverterOptions_DefaultEmpty(t *testing.T) {
	b := New(&mongoconfig.Config{Database: "x"})
	require.Empty(t, b.converterOpts)
}

func TestClientOptions(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *mongoconfig.Config
		wantErr bool
	}{
		{
			"valid basic",
			&mongoconfig.Config{
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
			&mongoconfig.Config{
				Hosts:      []string{"host1:27017", "host2:27017"},
				Database:   "testdb",
				ReplicaSet: "rs0",
			},
			false,
		},
		{
			"with compressors",
			&mongoconfig.Config{
				Hosts:                []string{"localhost:27017"},
				Database:             "testdb",
				Compressors:          mongoconfig.CompressionTypes{"snappy"},
				ZlibCompressionLevel: -1,
			},
			false,
		},
		{
			"with X509 auth",
			&mongoconfig.Config{
				Hosts:    []string{"localhost:27017"},
				Database: "testdb",
				Credentials: &mongoconfig.Credentials{
					AuthMechanism: mongoconfig.AuthMechanismTypeX509,
				},
			},
			false,
		},
		{
			"with PLAIN auth",
			&mongoconfig.Config{
				Hosts:    []string{"localhost:27017"},
				Database: "testdb",
				Credentials: &mongoconfig.Credentials{
					AuthMechanism: mongoconfig.AuthMechanismTypePLAIN,
					Plain: &mongoconfig.PLAINCredentials{
						Username: "user",
						Password: "pass",
					},
				},
			},
			false,
		},
		{
			"with SCRAM-SHA-256 auth",
			&mongoconfig.Config{
				Hosts:    []string{"localhost:27017"},
				Database: "testdb",
				Credentials: &mongoconfig.Credentials{
					AuthMechanism: mongoconfig.AuthMechanismTypeSCRAMSHA256,
					Scram: &mongoconfig.SCRAMCredentials{
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
	b := New(&mongoconfig.Config{
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
	b := New(&mongoconfig.Config{
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
		creds    *mongoconfig.Credentials
		wantMech string
	}{
		{
			"nil creds",
			nil,
			"",
		},
		{
			"X509",
			&mongoconfig.Credentials{AuthMechanism: mongoconfig.AuthMechanismTypeX509},
			"MONGODB-X509",
		},
		{
			"PLAIN",
			&mongoconfig.Credentials{
				AuthMechanism: mongoconfig.AuthMechanismTypePLAIN,
				Plain:         &mongoconfig.PLAINCredentials{Username: "u", Password: "p"},
			},
			"PLAIN",
		},
		{
			"PLAIN nil creds",
			&mongoconfig.Credentials{AuthMechanism: mongoconfig.AuthMechanismTypePLAIN},
			"",
		},
		{
			"SCRAM-SHA-1",
			&mongoconfig.Credentials{
				AuthMechanism: mongoconfig.AuthMechanismTypeSCRAMSHA1,
				Scram:         &mongoconfig.SCRAMCredentials{Username: "u", Password: "p", AuthSource: "admin"},
			},
			"SCRAM-SHA-1",
		},
		{
			"SCRAM nil creds",
			&mongoconfig.Credentials{AuthMechanism: mongoconfig.AuthMechanismTypeSCRAMSHA1},
			"",
		},
		{
			"unknown mechanism",
			&mongoconfig.Credentials{AuthMechanism: mongoconfig.AuthMechanismType("unknown")},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New(&mongoconfig.Config{Credentials: tt.creds})
			cred := b.buildCredential()
			require.Equal(t, tt.wantMech, cred.AuthMechanism)
		})
	}
}
