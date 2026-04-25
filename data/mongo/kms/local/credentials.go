// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmslocal

import (
	"github.com/altessa-s/go-atlas/core/text/strings"
)

// LocalCredentials provides secure storage for local KMS credentials.
type LocalCredentials struct {
	MasterKey *strings.SecureString
}

// NewLocalCredentials creates a new LocalCredentials instance with secure storage.
//
// Parameters:
//   - masterKey: Local master key data (will be stored securely)
//
// Returns:
//   - *LocalCredentials: New local credentials instance with secure storage
func NewLocalCredentials(masterKey []byte) *LocalCredentials {
	return &LocalCredentials{
		MasterKey: strings.NewSecureString(string(masterKey)),
	}
}

// Clear clears all stored credentials and zeros sensitive memory.
func (c *LocalCredentials) Clear() {
	if c.MasterKey != nil {
		c.MasterKey.Clear()
		c.MasterKey = nil
	}
}
