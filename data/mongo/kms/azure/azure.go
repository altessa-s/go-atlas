// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsazure

import (
	"crypto/tls"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/data/mongo/kms"
)

// Azure represents an Azure Key Vault provider configuration.
// It implements the kms.Provider, kms.MasterKey, and kms.TLSConfig interfaces.
// Credentials are stored securely using the credentials package.
type Azure struct {
	credentialsCache atomic.Pointer[kms.Credentials]
	// credentials stores sensitive Azure AD authentication data securely
	credentials *AzureCredentials
	// KeyName is the name of the key in Azure Key Vault
	KeyName string
	// KeyVersion is the optional version of the key
	KeyVersion *string
	// KeyVaultEndpoint is the optional custom Key Vault endpoint URL
	KeyVaultEndpoint *string
	// TLS is the optional custom TLS configuration
	TLS *tls.Config
}

// New creates a new Azure Key Vault provider with the specified credentials and key.
// Credentials are stored securely and automatically cleared when no longer needed.
//
// Parameters:
//   - clientID: Azure AD application client ID (will be stored securely)
//   - clientSecret: Azure AD application client secret (will be stored securely)
//   - tenantID: Azure AD tenant ID (will be stored securely)
//   - keyName: Name of the key in Azure Key Vault
//   - opts: Optional configuration options
//
// Example:
//
//	azure := kmsazure.New("client-id", "client-secret", "tenant-id", "my-key",
//		kmsazure.WithKeyVersion("1"),
//		kmsazure.WithKeyVaultEndpoint("https://vault.vault.azure.net"))
func New(clientID, clientSecret, tenantID, keyName string, opts ...Option) *Azure {
	options := newOptions(opts...)

	azure := &Azure{
		credentials:      NewAzureCredentials(clientID, clientSecret, tenantID),
		KeyName:          keyName,
		KeyVersion:       options.keyVersion,
		KeyVaultEndpoint: options.keyVaultEndpoint,
		TLS:              options.tLS,
	}

	return azure
}

// Name returns the provider name "azure".
// This implements the kms.Provider interface.
func (az *Azure) Name() string {
	return ProviderName
}

// Credentials returns the Azure AD authentication credentials.
// This implements the kms.Provider interface.
// Note: This method exposes sensitive credentials - use with caution.
func (az *Azure) Credentials() kms.Credentials {
	if cred := az.credentialsCache.Load(); cred != nil {
		return *cred
	}

	if az.credentials == nil {
		return kms.Credentials{az.Name(): {}}
	}

	k := kms.Credentials{
		az.Name(): {
			TenantID:     az.credentials.TenantID.StringUnsafe(),
			ClientID:     az.credentials.ClientID.StringUnsafe(),
			ClientSecret: az.credentials.ClientSecret.StringUnsafe(),
		},
	}

	az.credentialsCache.Store(&k)
	return k
}

// MasterKey returns the Azure Key Vault master key configuration.
// This implements the kms.MasterKey interface.
func (az *Azure) MasterKey() kms.Key {
	key := kms.Key{AzureKeyName: az.KeyName, AzureKeyVaultEndpoint: az.KeyVaultEndpoint}

	if az.KeyVersion != nil {
		key[AzureKeyVersion] = *az.KeyVersion
	}

	return key
}

// TLSConfig returns the TLS configuration for Azure Key Vault connections.
// This implements the simplified kms.Provider interface.
func (az *Azure) TLSConfig() *tls.Config {
	return az.TLS
}

// Clear securely clears all stored credentials and sensitive data.
// This method should be called when the Azure KMS provider is no longer needed
// to ensure sensitive credentials don't remain in memory.
func (az *Azure) Clear() {
	if az.credentials != nil {
		az.credentials.Clear()
		az.credentials = nil
	}
}
