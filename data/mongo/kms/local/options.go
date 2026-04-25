// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmslocal

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"os"
)

// options contains local key management provider configuration.
// Note: This struct is kept minimal because credential handling requires
// special logic that cannot be easily generated.
type options struct {
	// MasterKey holds the master key data directly.
	// The master key should be exactly 96 bytes long for proper AES-256 encryption.
	masterKey string

	// MasterKeyFile is the path to a file containing the master key.
	// The file should contain exactly 96 bytes of key data.
	// This is mutually exclusive with MasterKey.
	masterKeyFile string
}

// applyToLocal applies the options to a Local KMS provider instance.
// This handles the special credential logic that requires secure string creation.
func (o *options) applyToLocal(l *Local) {
	// MasterKeyFile takes precedence if both are set
	if o.masterKeyFile != "" {
		key, err := os.ReadFile(o.masterKeyFile) // #nosec G304 -- path from trusted config
		if err == nil && len(key) == RequiredMasterKeyLength {
			if l.credentials != nil {
				l.credentials.Clear()
			}
			l.credentials = NewLocalCredentials(key)
			return
		}
		// If file read fails or key length is invalid, fall through to try MasterKey
	}

	// Use MasterKey if provided
	if o.masterKey != "" {
		if l.credentials != nil {
			l.credentials.Clear()
		}
		l.credentials = NewLocalCredentials([]byte(o.masterKey))
	}
}
