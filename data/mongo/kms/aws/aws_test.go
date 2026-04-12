// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsaws_test

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/mongo/kms"

	kmsaws "github.com/altessa-s/go-atlas/data/mongo/kms/aws"
)

func TestNew(t *testing.T) {
	p := kmsaws.New("AKIAIOSFODNN7EXAMPLE", "secret", "arn:aws:kms:us-east-1:123:key/abc")
	require.NotNil(t, p)
}

func TestAmazon_Name(t *testing.T) {
	p := kmsaws.New("key", "secret", "arn")
	require.Equal(t, "aws", p.Name())
}

func TestAmazon_Credentials(t *testing.T) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	creds := p.Credentials()

	awsCreds, ok := creds["aws"]
	require.True(t, ok, "Credentials() missing 'aws' key")
	require.Equal(t, "AKID", awsCreds[kmsaws.AccessKeyID])
	require.Equal(t, "SECRET", awsCreds[kmsaws.SecretAccessKey])
}

func TestAWSCredentials_WithSessionToken(t *testing.T) {
	token := "session-token"
	creds := kmsaws.NewAWSCredentials("AKID", "SECRET", &token)
	require.NotNil(t, creds.SessionToken)
	require.Equal(t, "session-token", creds.SessionToken.StringUnsafe())
}

func TestAWSCredentials_Clear(t *testing.T) {
	creds := kmsaws.NewAWSCredentials("AKID", "SECRET", nil)
	creds.Clear()
	require.Nil(t, creds.AccessKeyID)
	require.Nil(t, creds.SecretAccessKey)
}

func TestAmazon_Credentials_Cached(t *testing.T) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	c1 := p.Credentials()
	c2 := p.Credentials()
	require.Equal(t, c1["aws"][kmsaws.AccessKeyID], c2["aws"][kmsaws.AccessKeyID])
}

func TestAmazon_MasterKey(t *testing.T) {
	region := "us-east-1"
	endpoint := "https://kms.custom.com"
	p := kmsaws.New("AKID", "SECRET", "arn:aws:kms:us-east-1:123:key/abc",
		kmsaws.WithRegion(&region),
		kmsaws.WithEndpoint(&endpoint),
	)

	key := p.MasterKey()
	require.Equal(t, "arn:aws:kms:us-east-1:123:key/abc", key[kmsaws.KeyARN])
	require.Equal(t, "us-east-1", key[kmsaws.Region])
	require.Equal(t, "https://kms.custom.com", key[kmsaws.Endpoint])
}

func TestAmazon_MasterKey_NoOptionals(t *testing.T) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	key := p.MasterKey()
	_, hasRegion := key[kmsaws.Region]
	require.False(t, hasRegion)
	_, hasEndpoint := key[kmsaws.Endpoint]
	require.False(t, hasEndpoint)
}

func TestAmazon_TLSConfig_Nil(t *testing.T) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	require.Nil(t, p.TLSConfig())
}

func TestAmazon_TLSConfig_Custom(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13} //nolint:gosec
	p := kmsaws.New("AKID", "SECRET", "arn", kmsaws.WithTLS(tlsCfg))
	require.Equal(t, tlsCfg, p.TLSConfig())
}

func TestAmazon_Clear(t *testing.T) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	p.Clear()
	creds := p.Credentials()
	require.Len(t, creds["aws"], 0)
}

func TestAmazon_ImplementsProvider(t *testing.T) {
	var _ kms.Provider = kmsaws.New("k", "s", "a")
}

func TestAmazon_HasMasterKey(t *testing.T) {
	p := kmsaws.New("k", "s", "a")
	require.True(t, kms.HasMasterKey(p))
}

func TestAmazon_HasCustomTLS(t *testing.T) {
	p := kmsaws.New("k", "s", "a")
	require.False(t, kms.HasCustomTLS(p))
}
