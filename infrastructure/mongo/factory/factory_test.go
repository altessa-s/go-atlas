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
	f := New()
	if f == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithOptions(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	coord := health.New()
	defer coord.Close()

	f := New(WithLogger(logger), WithHealthCoordinator(coord))
	if f == nil {
		t.Fatal("New() returned nil")
	}
}

func TestClientOptionsFromConfig(t *testing.T) {
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
			f := New()
			opts, err := f.ClientOptionsFromConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && opts == nil {
				t.Error("expected non-nil options")
			}
		})
	}
}

func TestClientOptionsFromConfig_ConnectionURI(t *testing.T) {
	f := New()
	cfg := &config.Mongodb{
		ConnectionURI:  "mongodb://user:pass@mongo1:27017,mongo2:27017/testdb?replicaSet=rs0",
		Database:       "testdb",
		MaxPoolSize:    200,
		MinPoolSize:    10,
		ConnectTimeout: 15 * time.Second,
		MaxIdleTimeout: 30 * time.Second,
		RetryReads:     true,
		RetryWrites:    true,
	}
	opts, err := f.ClientOptionsFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
}

func TestClientOptionsFromConfig_ConnectionURI_PoolOverlay(t *testing.T) {
	f := New()
	cfg := &config.Mongodb{
		ConnectionURI:  "mongodb://localhost:27017/testdb",
		Database:       "testdb",
		MaxPoolSize:    50,
		MinPoolSize:    5,
		ConnectTimeout: 10 * time.Second,
		RetryReads:     false,
		RetryWrites:    false,
	}
	opts, err := f.ClientOptionsFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
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
			f := New()
			cred := f.buildCredential(tt.creds)
			if cred.AuthMechanism != tt.wantMech {
				t.Errorf("AuthMechanism = %q, want %q", cred.AuthMechanism, tt.wantMech)
			}
		})
	}
}
