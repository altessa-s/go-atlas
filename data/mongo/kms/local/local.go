// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmslocal

import (
	"crypto/tls"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/data/mongo/kms"
)

// Local represents a local key management provider configuration.
// It implements the kms.Provider interface and optionally the kms.TLSConfig interface.
// Unlike cloud KMS providers, it does not implement kms.MasterKey as the master key
// is stored locally and used directly for encryption operations.
// Master key data is stored securely using the credentials package.
type Local struct {
	credentialsCache atomic.Pointer[kms.Credentials]
	// credentials stores the sensitive master key data securely
	credentials *LocalCredentials
}

// New creates a new local key management provider with key validation.
//
// The local provider requires at least one of WithMasterKey or WithMasterKeyFile
// options to be provided to set the master key. The master key must be exactly
// 96 bytes long as required by MongoDB Client-Side Field Level Encryption (CSFLE).
//
// Parameters:
//   - opts: Configuration options
//
// Returns:
//   - *Local: Local KMS provider instance
//   - error: Error if master key is missing or invalid length
//
// Example:
//
//	// Using direct master key
//	local, err := kmslocal.New(
//		kmslocal.WithMasterKey("my-96-byte-master-key-data-here-exactly-96-bytes-long-for-encryption-purposes"))
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Using master key from file
//	local, err := kmslocal.New(
//		kmslocal.WithMasterKeyFile("/path/to/masterkey.bin"))
//	if err != nil {
//		log.Fatal(err)
//	}
func New(opts ...Option) (*Local, error) {
	options := newOptions(opts...)
	l := &Local{}

	// Apply options with special credential handling
	options.applyToLocal(l)

	// Validate master key using structured error types
	if l.credentials == nil || l.credentials.MasterKey == nil || l.credentials.MasterKey.IsEmpty() {
		return nil, kms.NewMissingCredentialError(ProviderName, "masterKey")
	}

	if l.credentials.MasterKey.Len() != RequiredMasterKeyLength {
		return nil, kms.NewKeyLengthError(ProviderName,
			l.credentials.MasterKey.Len(), RequiredMasterKeyLength)
	}

	return l, nil
}

// Name returns the provider name "local".
// This implements the kms.Provider interface.
func (l *Local) Name() string {
	return ProviderName
}

// Credentials returns the local master key as credentials.
// This implements the kms.Provider interface.
// For local KMS, the "credentials" contain the actual master key data.
//
// Note: This method exposes sensitive key material - use with caution.
// The key has been validated during New() for proper length.
func (l *Local) Credentials() kms.Credentials {
	if cred := l.credentialsCache.Load(); cred != nil {
		return *cred
	}

	if l.credentials == nil || l.credentials.MasterKey == nil {
		return kms.Credentials{l.Name(): {}}
	}

	k := kms.Credentials{
		l.Name(): {MasterKey: l.credentials.MasterKey.Bytes()},
	}
	l.credentialsCache.Store(&k)
	return k
}

// TLSConfig returns the TLS configuration for local KMS operations.
// This implements the simplified kms.Provider interface.
func (l *Local) TLSConfig() *tls.Config {
	return nil
}

// MasterKey returns the local master key configuration.
// This implements the simplified kms.Provider interface.
func (l *Local) MasterKey() kms.Key {
	return nil
}

// Clear securely clears all stored master key data and sensitive information.
// This method should be called when the Local KMS provider is no longer needed
// to ensure sensitive key material doesn't remain in memory.
func (l *Local) Clear() {
	if l.credentials != nil {
		l.credentials.Clear()
		l.credentials = nil
	}
}
