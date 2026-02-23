// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsgcp

import (
	"github.com/altessa-s/go-atlas/core/text/strings"
)

// GCPCredentials provides secure storage for Google Cloud KMS credentials.
type GCPCredentials struct {
	Email      *strings.SecureString
	PrivateKey *strings.SecureString
}

// NewGCPCredentials creates a new GCPCredentials instance with secure storage.
//
// Parameters:
//   - email: GCP service account email (will be stored securely)
//   - privateKey: GCP service account private key (will be stored securely)
//
// Returns:
//   - *GCPCredentials: New GCP credentials instance with secure storage
func NewGCPCredentials(email, privateKey string) *GCPCredentials {
	return &GCPCredentials{
		Email:      strings.NewSecureString(email),
		PrivateKey: strings.NewSecureString(privateKey),
	}
}

// Clear clears all stored credentials and zeros sensitive memory.
func (c *GCPCredentials) Clear() {
	if c.Email != nil {
		c.Email.Clear()
		c.Email = nil
	}

	if c.PrivateKey != nil {
		c.PrivateKey.Clear()
		c.PrivateKey = nil
	}
}
