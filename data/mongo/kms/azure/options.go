// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsazure

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"crypto/tls"
)

// options contains Azure Key Vault provider configuration.
type options struct {
	// KeyVersion sets the version of the key in Azure Key Vault.
	// This is optional and allows using a specific version of the key.
	// If not specified, the latest version will be used.
	keyVersion *string

	// KeyVaultEndpoint sets a custom Azure Key Vault endpoint URL.
	// This is typically used when working with different Azure environments.
	// Example: "https://myvault.vault.azure.net"
	keyVaultEndpoint *string

	// TLS sets a custom TLS configuration for connections to Azure Key Vault.
	// This allows fine-grained control over TLS settings for secure connections.
	tLS *tls.Config
}
