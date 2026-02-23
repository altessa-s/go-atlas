// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"
	"time"
)

func TestValidate_Auth_NilSubConfigs(t *testing.T) {
	cfg := Auth{}
	// Auth with nil sub-configs should still pass (NilOrNotEmpty)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid for empty Auth: %v", err)
	}
}

func TestValidate_Grpc_Valid(t *testing.T) {
	cfg := DefaultGrpc()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid: %v", err)
	}
}

func TestValidate_Grpc_InvalidAddress(t *testing.T) {
	cfg := DefaultGrpc()
	cfg.ListenAddress = "not-valid"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid address")
	}
}

func TestValidate_Http_Valid(t *testing.T) {
	cfg := DefaultHttp()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid: %v", err)
	}
}

func TestValidate_Http_InvalidAddress(t *testing.T) {
	cfg := DefaultHttp()
	cfg.ListenAddress = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidate_Http_NegativePayload(t *testing.T) {
	cfg := DefaultHttp()
	cfg.MaxRequestPayloadSize = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for negative payload size")
	}
}

func TestValidate_Mongodb_Valid(t *testing.T) {
	cfg := DefaultMongodb()
	cfg.Database = "testdb"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid: %v", err)
	}
}

func TestValidate_Mongodb_NoHosts(t *testing.T) {
	cfg := DefaultMongodb()
	cfg.Database = "testdb"
	cfg.Hosts = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty hosts")
	}
}

func TestValidate_Mongodb_NoDatabase(t *testing.T) {
	cfg := DefaultMongodb()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty database")
	}
}

func TestValidate_Redis_Valid(t *testing.T) {
	cfg := DefaultRedis()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid: %v", err)
	}
}

func TestValidate_Redis_NoHosts(t *testing.T) {
	cfg := DefaultRedis()
	cfg.Hosts = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidate_Nats_Valid(t *testing.T) {
	cfg := DefaultNats()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid: %v", err)
	}
}

func TestValidate_Nats_NoHosts(t *testing.T) {
	cfg := DefaultNats()
	cfg.Hosts = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidate_Nats_ZeroPingInterval(t *testing.T) {
	cfg := DefaultNats()
	cfg.PingInterval = 0 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for zero ping interval")
	}
}

// --- ConnectionURI validation tests ---

func TestValidate_Nats_ConnectionURI_Only(t *testing.T) {
	cfg := DefaultNats()
	cfg.ConnectionURI = "nats://user:pass@nats:4222"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid with URI only: %v", err)
	}
}

func TestValidate_Nats_ConnectionURI_NoHosts(t *testing.T) {
	cfg := Nats{
		ConnectionURI:  "nats://nats:4222",
		PingInterval:   10 * time.Second,
		ReconnectWait:  10 * time.Second,
		ConnectTimeout: 5 * time.Second,
		MaxPingsOut:    3,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid with URI and no hosts: %v", err)
	}
}

func TestValidate_Nats_ConnectionURI_ConflictUsername(t *testing.T) {
	cfg := DefaultNats()
	cfg.ConnectionURI = "nats://nats:4222"
	cfg.Username = "user"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflict error for URI + username")
	}
}

func TestValidate_Nats_ConnectionURI_ConflictPassword(t *testing.T) {
	cfg := DefaultNats()
	cfg.ConnectionURI = "nats://nats:4222"
	cfg.Password = "pass"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflict error for URI + password")
	}
}

func TestValidate_Nats_ConnectionURI_ConflictToken(t *testing.T) {
	cfg := DefaultNats()
	cfg.ConnectionURI = "nats://nats:4222"
	cfg.Token = "tok"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflict error for URI + token")
	}
}

func TestValidate_Nats_ConnectionURI_ConflictNkeySeed(t *testing.T) {
	cfg := DefaultNats()
	cfg.ConnectionURI = "nats://nats:4222"
	cfg.NkeySeed = "SUACSSL3UAHUDXKFSNVUZRF5UHPMWZ6BFDTJ7M6USDXIEDNPPQYYYCU3VY"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflict error for URI + nkeySeed")
	}
}

func TestValidate_Nats_ConnectionURI_MultipleConflicts(t *testing.T) {
	cfg := DefaultNats()
	cfg.ConnectionURI = "nats://nats:4222"
	cfg.Username = "user"
	cfg.Token = "tok"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected conflict error for URI + multiple auth fields")
	}
}

func TestValidate_Redis_ConnectionURI_Only(t *testing.T) {
	cfg := DefaultRedis()
	cfg.ConnectionURI = "redis://localhost:6379/0"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid with URI only: %v", err)
	}
}

func TestValidate_Redis_ConnectionURI_NoHosts(t *testing.T) {
	cfg := Redis{
		ConnectionURI:  "redis://localhost:6379/0",
		ConnectTimeout: 5 * time.Second,
		SocketTimeout:  5 * time.Second,
		IdleTimeout:    5 * time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid with URI and no hosts: %v", err)
	}
}

func TestValidate_Redis_ConnectionURI_ConflictUsername(t *testing.T) {
	cfg := DefaultRedis()
	cfg.ConnectionURI = "redis://localhost:6379/0"
	cfg.Username = "user"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflict error for URI + username")
	}
}

func TestValidate_Redis_ConnectionURI_ConflictPassword(t *testing.T) {
	cfg := DefaultRedis()
	cfg.ConnectionURI = "redis://localhost:6379/0"
	cfg.Password = "pass"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflict error for URI + password")
	}
}

func TestValidate_Mongodb_ConnectionURI_Only(t *testing.T) {
	cfg := DefaultMongodb()
	cfg.ConnectionURI = "mongodb://localhost:27017/testdb"
	cfg.Database = "testdb"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid with URI only: %v", err)
	}
}

func TestValidate_Mongodb_ConnectionURI_NoHosts(t *testing.T) {
	cfg := Mongodb{
		ConnectionURI:  "mongodb://localhost:27017/testdb",
		Database:       "testdb",
		ConnectTimeout: 30 * time.Second,
		MaxPoolSize:    100,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid with URI and no hosts: %v", err)
	}
}

func TestValidate_Mongodb_ConnectionURI_DatabaseRequired(t *testing.T) {
	cfg := DefaultMongodb()
	cfg.ConnectionURI = "mongodb://localhost:27017/testdb"
	// Database is still required even with URI
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty database even with URI")
	}
}

func TestValidate_Mongodb_ConnectionURI_ConflictCredentials(t *testing.T) {
	cfg := DefaultMongodb()
	cfg.ConnectionURI = "mongodb://user:pass@localhost:27017/testdb"
	cfg.Database = "testdb"
	cfg.Credentials = &MongodbCredentials{
		AuthMechanism: MongoAuthMechanismTypePLAIN,
		Plain:         &MongoPLAINCredentials{Username: "user", Password: "pass"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflict error for URI + credentials")
	}
}

func TestValidate_Nats_UseConnectionURI(t *testing.T) {
	n := &Nats{}
	if n.UseConnectionURI() {
		t.Fatal("empty URI should return false")
	}
	n.ConnectionURI = "nats://localhost:4222"
	if !n.UseConnectionURI() {
		t.Fatal("non-empty URI should return true")
	}
}

func TestValidate_Redis_UseConnectionURI(t *testing.T) {
	r := &Redis{}
	if r.UseConnectionURI() {
		t.Fatal("empty URI should return false")
	}
	r.ConnectionURI = "redis://localhost:6379"
	if !r.UseConnectionURI() {
		t.Fatal("non-empty URI should return true")
	}
}

func TestValidate_Mongodb_UseConnectionURI(t *testing.T) {
	m := &Mongodb{}
	if m.UseConnectionURI() {
		t.Fatal("empty URI should return false")
	}
	m.ConnectionURI = "mongodb://localhost:27017"
	if !m.UseConnectionURI() {
		t.Fatal("non-empty URI should return true")
	}
}
