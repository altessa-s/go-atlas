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

// Provider represents a unified KMS provider interface.
// This interface replaces the complex hierarchy of Provider, MasterKey, TLSConfig, etc.
// All providers implement this single interface, using nil returns for unsupported features.
type Provider interface {
	// Core provider functionality (required)
	Name() string
	Credentials() Credentials
	Clear()

	// Optional functionality (can return nil if not supported)
	MasterKey() Key         // Returns nil if provider doesn't support master keys
	TLSConfig() *tls.Config // Returns nil if provider doesn't need custom TLS
}

// Helper functions for capability checking (simplified)

// HasMasterKey returns true if the provider supports master key functionality
func HasMasterKey(p Provider) bool {
	return p.MasterKey() != nil
}

// HasCustomTLS returns true if the provider requires custom TLS configuration
func HasCustomTLS(p Provider) bool {
	return p.TLSConfig() != nil
}

// IsFullyFeatured returns true if the provider supports all optional features
func IsFullyFeatured(p Provider) bool {
	return HasMasterKey(p) && HasCustomTLS(p)
}
