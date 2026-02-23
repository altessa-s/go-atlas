// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsazure_test

import (
	"crypto/tls"
	"testing"

	"github.com/altessa-s/go-atlas/data/mongo/kms"

	kmsazure "github.com/altessa-s/go-atlas/data/mongo/kms/azure"
)

func TestNew(t *testing.T) {
	p := kmsazure.New("client-id", "client-secret", "tenant-id", "my-key")
	if p == nil {
		t.Fatal("New() returned nil")
	}
}

func TestAzure_Name(t *testing.T) {
	p := kmsazure.New("c", "s", "t", "k")
	if p.Name() != "azure" {
		t.Errorf("Name() = %q, want %q", p.Name(), "azure")
	}
}

func TestAzure_Credentials(t *testing.T) {
	p := kmsazure.New("client-id", "client-secret", "tenant-id", "key")
	creds := p.Credentials()

	azCreds, ok := creds["azure"]
	if !ok {
		t.Fatal("Credentials() missing 'azure' key")
	}
	if azCreds[kmsazure.ClientID] != "client-id" {
		t.Errorf("ClientID = %v", azCreds[kmsazure.ClientID])
	}
	if azCreds[kmsazure.ClientSecret] != "client-secret" {
		t.Errorf("ClientSecret = %v", azCreds[kmsazure.ClientSecret])
	}
	if azCreds[kmsazure.TenantID] != "tenant-id" {
		t.Errorf("TenantID = %v", azCreds[kmsazure.TenantID])
	}
}

func TestAzure_MasterKey(t *testing.T) {
	version := "1"
	endpoint := "https://vault.vault.azure.net"
	p := kmsazure.New("c", "s", "t", "my-key",
		kmsazure.WithKeyVersion(&version),
		kmsazure.WithKeyVaultEndpoint(&endpoint),
	)

	key := p.MasterKey()
	if key[kmsazure.AzureKeyName] != "my-key" {
		t.Errorf("KeyName = %v", key[kmsazure.AzureKeyName])
	}
	if key[kmsazure.AzureKeyVersion] != "1" {
		t.Errorf("KeyVersion = %v", key[kmsazure.AzureKeyVersion])
	}
}

func TestAzure_MasterKey_NoOptionals(t *testing.T) {
	p := kmsazure.New("c", "s", "t", "key")
	key := p.MasterKey()
	if _, ok := key[kmsazure.AzureKeyVersion]; ok {
		t.Error("KeyVersion should not be set")
	}
}

func TestAzure_TLSConfig_Nil(t *testing.T) {
	p := kmsazure.New("c", "s", "t", "k")
	if p.TLSConfig() != nil {
		t.Error("TLSConfig() should be nil by default")
	}
}

func TestAzure_TLSConfig_Custom(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13} //nolint:gosec
	p := kmsazure.New("c", "s", "t", "k", kmsazure.WithTLS(tlsCfg))
	if p.TLSConfig() != tlsCfg {
		t.Error("TLSConfig() should return custom config")
	}
}

func TestAzure_Clear(t *testing.T) {
	p := kmsazure.New("c", "s", "t", "k")
	p.Clear()
	creds := p.Credentials()
	if len(creds["azure"]) != 0 {
		t.Errorf("Credentials() after Clear() should be empty, got %v", creds["azure"])
	}
}

func TestAzure_ImplementsProvider(t *testing.T) {
	var _ kms.Provider = kmsazure.New("c", "s", "t", "k")
}
