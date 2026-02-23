// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsgcp_test

import (
	"crypto/tls"
	"testing"

	"github.com/altessa-s/go-atlas/data/mongo/kms"

	kmsgcp "github.com/altessa-s/go-atlas/data/mongo/kms/gcp"
)

func TestNew(t *testing.T) {
	p := kmsgcp.New("project", "email@sa.com", "privkey", "global", "ring", "key")
	if p == nil {
		t.Fatal("New() returned nil")
	}
}

func TestGoogle_Name(t *testing.T) {
	p := kmsgcp.New("p", "e", "k", "l", "r", "n")
	if p.Name() != "gcp" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gcp")
	}
}

func TestGoogle_Credentials(t *testing.T) {
	p := kmsgcp.New("project", "email@test.com", "private-key", "global", "ring", "key")
	creds := p.Credentials()

	gcpCreds, ok := creds["gcp"]
	if !ok {
		t.Fatal("Credentials() missing 'gcp' key")
	}
	if gcpCreds[kmsgcp.Email] != "email@test.com" {
		t.Errorf("Email = %v", gcpCreds[kmsgcp.Email])
	}
	if gcpCreds[kmsgcp.GCPPrivateKey] != "private-key" {
		t.Errorf("PrivateKey = %v", gcpCreds[kmsgcp.GCPPrivateKey])
	}
}

func TestGoogle_Credentials_WithAuthEndpoint(t *testing.T) {
	authEp := "https://custom-auth.com"
	p := kmsgcp.New("p", "e", "k", "l", "r", "n",
		kmsgcp.WithAuthenticationEndpoint(&authEp),
	)
	creds := p.Credentials()
	if creds["gcp"][kmsgcp.Endpoint] != "https://custom-auth.com" {
		t.Errorf("Endpoint = %v", creds["gcp"][kmsgcp.Endpoint])
	}
}

func TestGoogle_MasterKey(t *testing.T) {
	version := "1"
	endpoint := "https://custom.kms.com"
	p := kmsgcp.New("project", "e", "k", "global", "ring", "key",
		kmsgcp.WithKeyVersion(&version),
		kmsgcp.WithEndpoint(&endpoint),
	)

	key := p.MasterKey()
	if key[kmsgcp.ProjectID] != "project" {
		t.Errorf("ProjectID = %v", key[kmsgcp.ProjectID])
	}
	if key[kmsgcp.GCPLocation] != "global" {
		t.Errorf("Location = %v", key[kmsgcp.GCPLocation])
	}
	if key[kmsgcp.KeyRing] != "ring" {
		t.Errorf("KeyRing = %v", key[kmsgcp.KeyRing])
	}
	if key[kmsgcp.GCPKeyName] != "key" {
		t.Errorf("KeyName = %v", key[kmsgcp.GCPKeyName])
	}
	if key[kmsgcp.KeyVersion] != "1" {
		t.Errorf("KeyVersion = %v", key[kmsgcp.KeyVersion])
	}
	if key[kmsgcp.Endpoint] != "https://custom.kms.com" {
		t.Errorf("Endpoint = %v", key[kmsgcp.Endpoint])
	}
}

func TestGoogle_MasterKey_NoOptionals(t *testing.T) {
	p := kmsgcp.New("p", "e", "k", "l", "r", "n")
	key := p.MasterKey()
	if _, ok := key[kmsgcp.KeyVersion]; ok {
		t.Error("KeyVersion should not be set")
	}
	if _, ok := key[kmsgcp.Endpoint]; ok {
		t.Error("Endpoint should not be set")
	}
}

func TestGoogle_TLSConfig_Nil(t *testing.T) {
	p := kmsgcp.New("p", "e", "k", "l", "r", "n")
	if p.TLSConfig() != nil {
		t.Error("TLSConfig() should be nil by default")
	}
}

func TestGoogle_TLSConfig_Custom(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13} //nolint:gosec
	p := kmsgcp.New("p", "e", "k", "l", "r", "n", kmsgcp.WithTLS(tlsCfg))
	if p.TLSConfig() != tlsCfg {
		t.Error("TLSConfig() should return custom config")
	}
}

func TestGoogle_Clear(t *testing.T) {
	p := kmsgcp.New("p", "e", "k", "l", "r", "n")
	p.Clear()
	creds := p.Credentials()
	if len(creds["gcp"]) != 0 {
		t.Errorf("Credentials() after Clear() should be empty, got %v", creds["gcp"])
	}
}

func TestGoogle_ImplementsProvider(t *testing.T) {
	var _ kms.Provider = kmsgcp.New("p", "e", "k", "l", "r", "n")
}
