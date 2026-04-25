// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockbox

import (
	"errors"
	"os"
)

// ReadPrivateKey reads the private key content for Yandex Cloud IAM authentication.
// It tries to read the key from either a provided string or a file path.
//
// The function first checks if privateKeyData is provided and non-nil. If so, it returns
// the contents of that string as a byte slice. Otherwise, it attempts to read the key
// from the file specified by path.
//
// Parameters:
//   - privateKeyData: pointer to a string containing the private key data (optional)
//   - path: pointer to a string containing the path to the private key file (used if privateKeyData is nil)
//
// Returns:
//   - The private key as a byte slice
//   - An error if both privateKeyData and path are nil/empty, or if the file could not be read
func ReadPrivateKey(privateKeyData *string, path *string) ([]byte, error) {
	if privateKeyData != nil {
		return []byte(*privateKeyData), nil
	}

	if path == nil || *path == "" {
		return nil, errors.New("private key data and path are empty")
	}

	privateKey, err := os.ReadFile(*path)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}
