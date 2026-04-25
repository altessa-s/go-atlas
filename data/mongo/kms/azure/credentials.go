// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsazure

import (
	"github.com/altessa-s/go-atlas/core/text/strings"
)

// AzureCredentials provides secure storage for Azure Key Vault credentials.
type AzureCredentials struct {
	ClientID     *strings.SecureString
	ClientSecret *strings.SecureString
	TenantID     *strings.SecureString
}

// NewAzureCredentials creates a new AzureCredentials instance with secure storage.
//
// Parameters:
//   - clientID: Azure AD client ID (will be stored securely)
//   - clientSecret: Azure AD client secret (will be stored securely)
//   - tenantID: Azure AD tenant ID (will be stored securely)
//
// Returns:
//   - *AzureCredentials: New Azure credentials instance with secure storage
func NewAzureCredentials(clientID, clientSecret, tenantID string) *AzureCredentials {
	return &AzureCredentials{
		ClientID:     strings.NewSecureString(clientID),
		ClientSecret: strings.NewSecureString(clientSecret),
		TenantID:     strings.NewSecureString(tenantID),
	}
}

// Clear clears all stored credentials and zeros sensitive memory.
func (c *AzureCredentials) Clear() {
	if c.ClientID != nil {
		c.ClientID.Clear()
		c.ClientID = nil
	}
	if c.ClientSecret != nil {
		c.ClientSecret.Clear()
		c.ClientSecret = nil
	}
	if c.TenantID != nil {
		c.TenantID.Clear()
		c.TenantID = nil
	}
}
