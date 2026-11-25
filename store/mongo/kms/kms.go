// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kms

import (
	"crypto/tls"
)

// Credentials is a map of provider name to a map of key name to value.
// It represents the authentication credentials for a KMS provider.
//
// Example structure:
//
//	{
//	  "aws": {
//	    "accessKeyId": "AKIA...",
//	    "secretAccessKey": "...",
//	    "sessionToken": "..." // optional
//	  }
//	}
type Credentials map[string]map[string]any

// Key is a map of key name to value that represents a master key configuration
// for a KMS provider. The structure varies by provider.
//
// Examples:
//   - AWS: {"key": "arn:aws:kms:region:account:key/key-id", "region": "us-east-1"}
//   - Azure: {"keyName": "mykey", "keyVaultEndpoint": "https://vault.vault.azure.net"}
//   - GCP: {"projectId": "my-project", "location": "global", "keyRing": "ring", "keyName": "key"}
type Key map[string]any

// MasterKeyProvider supplies the credentials and key descriptor used by MongoDB CSFLE.
type MasterKeyProvider interface {
	Name() string
	Credentials() Credentials
	Clear()
	// MasterKey returns the driver key descriptor. Local key material is supplied
	// in Credentials and needs no separate descriptor.
	MasterKey() Key
}

// TLSConfigurer supplies custom TLS settings for a remote KMS.
type TLSConfigurer interface {
	TLSConfig() *tls.Config
}

// HasMasterKey returns true if the provider supports master key functionality
func HasMasterKey(p MasterKeyProvider) bool {
	return p.MasterKey() != nil
}

// HasCustomTLS returns true if the provider requires custom TLS configuration
func HasCustomTLS(p MasterKeyProvider) bool {
	tlsProvider, ok := p.(TLSConfigurer)
	return ok && tlsProvider.TLSConfig() != nil
}

// IsFullyFeatured returns true if the provider supports all optional features
func IsFullyFeatured(p MasterKeyProvider) bool {
	return HasMasterKey(p) && HasCustomTLS(p)
}
