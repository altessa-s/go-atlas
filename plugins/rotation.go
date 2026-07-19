// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"crypto"
	"errors"
	"fmt"
	"os"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// MultiKeySignatureOptions configures plugin signature verification with
// multiple public keys for key rotation support. Pass it to
// [WithMultiKeySignature] when constructing the manager.
type MultiKeySignatureOptions struct {
	// Mode controls verification behavior. Default is [SignatureRequire].
	Mode SignatureMode

	// PublicKeys is a list of verification keys to try in order.
	// Signature verification succeeds if any key validates the signature.
	// Accepted types: [ed25519.PublicKey], [*ecdsa.PublicKey], [*rsa.PublicKey].
	PublicKeys []crypto.PublicKey

	// PublicKeyPaths is a list of paths to PEM-encoded public key files.
	// Used when PublicKeys is empty. Keys are tried in order.
	PublicKeyPaths []string
}

// loadMultiplePublicKeys loads public keys from multiple files.
func loadMultiplePublicKeys(opts MultiKeySignatureOptions) ([]crypto.PublicKey, error) {
	var keys []crypto.PublicKey
	var loadErrors []error

	for _, path := range opts.PublicKeyPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			loadErrors = append(loadErrors, coreerrs.Wrapf(err, "read key file %q", path))
			continue
		}

		key, err := parsePublicKeyPEM(data)
		if err != nil {
			loadErrors = append(loadErrors, coreerrs.Wrapf(err, "parse key file %q", path))
			continue
		}

		keys = append(keys, key)
	}

	// If all keys failed to load, return the combined error
	if len(keys) == 0 && len(loadErrors) > 0 {
		return nil, coreerrs.Wrapf(ErrSignatureConfig, "failed to load any public keys: %v", errors.Join(loadErrors...))
	}

	return keys, nil
}

// verifySignatureMultiKey tries to verify a signature with multiple keys.
// Returns nil if any key successfully verifies the signature.
func verifySignatureMultiKey(pubKeys []crypto.PublicKey, pluginData []byte, sigPath string) error {
	if len(pubKeys) == 0 {
		return fmt.Errorf("%w: no public keys configured", ErrSignatureConfig)
	}

	// Try single key fast path
	if len(pubKeys) == 1 {
		return verifySignature(pubKeys[0], pluginData, sigPath)
	}

	// Try each key in order
	var verifyErrors []error
	for i, key := range pubKeys {
		if err := verifySignature(key, pluginData, sigPath); err == nil {
			// Success with this key
			return nil
		} else if !errors.Is(err, ErrSignatureInvalid) && !errors.Is(err, os.ErrNotExist) {
			// Unexpected error (not just wrong signature or missing file)
			verifyErrors = append(verifyErrors, coreerrs.Wrapf(err, "key %d", i))
		}
	}

	// All keys failed
	if len(verifyErrors) > 0 {
		return coreerrs.Wrapf(errors.Join(verifyErrors...), "signature verification failed with all %d keys", len(pubKeys))
	}

	return ErrSignatureInvalid
}
