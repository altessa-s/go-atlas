// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsazure

// ProviderName is the identifier for Azure Key Vault provider
const ProviderName = "azure"

const (
	// TenantID is the credential field name for Azure tenant ID
	TenantID = "tenantId"

	// ClientID is the credential field name for Azure client ID
	ClientID = "clientId"

	// ClientSecret is the credential field name for Azure client secret
	ClientSecret = "clientSecret"
)

const (
	// AzureKeyName is the master key field name for Azure key name
	AzureKeyName = "keyName"

	// AzureKeyVersion is the master key field name for Azure key version (optional)
	AzureKeyVersion = "keyVersion"

	// AzureKeyVaultEndpoint is the master key field name for Azure Key Vault endpoint
	AzureKeyVaultEndpoint = "keyVaultEndpoint"
)
