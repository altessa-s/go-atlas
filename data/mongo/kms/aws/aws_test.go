// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsaws_test

import (
	"crypto/tls"
	"testing"

	"github.com/altessa-s/go-atlas/data/mongo/kms"

	kmsaws "github.com/altessa-s/go-atlas/data/mongo/kms/aws"
)

func TestNew(t *testing.T) {
	p := kmsaws.New("AKIAIOSFODNN7EXAMPLE", "secret", "arn:aws:kms:us-east-1:123:key/abc")
	if p == nil {
		t.Fatal("New() returned nil")
	}
}

func TestAmazon_Name(t *testing.T) {
	p := kmsaws.New("key", "secret", "arn")
	if p.Name() != "aws" {
		t.Errorf("Name() = %q, want %q", p.Name(), "aws")
	}
}

func TestAmazon_Credentials(t *testing.T) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	creds := p.Credentials()

	awsCreds, ok := creds["aws"]
	if !ok {
		t.Fatal("Credentials() missing 'aws' key")
	}
	if awsCreds[kmsaws.AccessKeyID] != "AKID" {
		t.Errorf("AccessKeyID = %v, want %q", awsCreds[kmsaws.AccessKeyID], "AKID")
	}
	if awsCreds[kmsaws.SecretAccessKey] != "SECRET" {
		t.Errorf("SecretAccessKey = %v, want %q", awsCreds[kmsaws.SecretAccessKey], "SECRET")
	}
}

func TestAWSCredentials_WithSessionToken(t *testing.T) {
	token := "session-token"
	creds := kmsaws.NewAWSCredentials("AKID", "SECRET", &token)
	if creds.SessionToken == nil {
		t.Fatal("SessionToken should not be nil")
	}
	if creds.SessionToken.StringUnsafe() != "session-token" {
		t.Errorf("SessionToken = %q, want %q", creds.SessionToken.StringUnsafe(), "session-token")
	}
}

func TestAWSCredentials_Clear(t *testing.T) {
	creds := kmsaws.NewAWSCredentials("AKID", "SECRET", nil)
	creds.Clear()
	if creds.AccessKeyID != nil {
		t.Error("AccessKeyID should be nil after Clear()")
	}
	if creds.SecretAccessKey != nil {
		t.Error("SecretAccessKey should be nil after Clear()")
	}
}

func TestAmazon_Credentials_Cached(t *testing.T) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	c1 := p.Credentials()
	c2 := p.Credentials()
	if c1["aws"][kmsaws.AccessKeyID] != c2["aws"][kmsaws.AccessKeyID] {
		t.Error("Credentials() should return consistent values")
	}
}

func TestAmazon_MasterKey(t *testing.T) {
	region := "us-east-1"
	endpoint := "https://kms.custom.com"
	p := kmsaws.New("AKID", "SECRET", "arn:aws:kms:us-east-1:123:key/abc",
		kmsaws.WithRegion(&region),
		kmsaws.WithEndpoint(&endpoint),
	)

	key := p.MasterKey()
	if key[kmsaws.KeyARN] != "arn:aws:kms:us-east-1:123:key/abc" {
		t.Errorf("KeyARN = %v", key[kmsaws.KeyARN])
	}
	if key[kmsaws.Region] != "us-east-1" {
		t.Errorf("Region = %v", key[kmsaws.Region])
	}
	if key[kmsaws.Endpoint] != "https://kms.custom.com" {
		t.Errorf("Endpoint = %v", key[kmsaws.Endpoint])
	}
}

func TestAmazon_MasterKey_NoOptionals(t *testing.T) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	key := p.MasterKey()
	if _, ok := key[kmsaws.Region]; ok {
		t.Error("Region should not be set")
	}
	if _, ok := key[kmsaws.Endpoint]; ok {
		t.Error("Endpoint should not be set")
	}
}

func TestAmazon_TLSConfig_Nil(t *testing.T) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	if p.TLSConfig() != nil {
		t.Error("TLSConfig() should be nil by default")
	}
}

func TestAmazon_TLSConfig_Custom(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13} //nolint:gosec
	p := kmsaws.New("AKID", "SECRET", "arn", kmsaws.WithTLS(tlsCfg))
	if p.TLSConfig() != tlsCfg {
		t.Error("TLSConfig() should return custom config")
	}
}

func TestAmazon_Clear(t *testing.T) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	p.Clear()
	// After clear, credentials should be empty
	creds := p.Credentials()
	if len(creds["aws"]) != 0 {
		t.Errorf("Credentials() after Clear() should be empty, got %v", creds["aws"])
	}
}

func TestAmazon_ImplementsProvider(t *testing.T) {
	var _ kms.Provider = kmsaws.New("k", "s", "a")
}

func TestAmazon_HasMasterKey(t *testing.T) {
	p := kmsaws.New("k", "s", "a")
	if !kms.HasMasterKey(p) {
		t.Error("AWS provider should have master key")
	}
}

func TestAmazon_HasCustomTLS(t *testing.T) {
	p := kmsaws.New("k", "s", "a")
	if kms.HasCustomTLS(p) {
		t.Error("AWS provider should not have custom TLS by default")
	}
}
