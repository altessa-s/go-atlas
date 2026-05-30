// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
)

const (
	algEd25519 = "ed25519"
	algECDSA   = "ecdsa"
	algRSA     = "rsa"
)

const (
	rsaMinBits     = 2048
	rsaDefaultBits = 3072
	rsaMaxBits     = 4096

	privFileMode os.FileMode = 0o600
	pubFileMode  os.FileMode = 0o644

	pemTypePrivateKey = "PRIVATE KEY"
	pemTypePublicKey  = "PUBLIC KEY"

	defaultPrivOut = "private.pem"
	defaultPubOut  = "public.pem"
)

// errKeyFileExists signals that a key output path already exists and -force
// was not passed. Surfaced as a distinct error so tests can match on it.
var errKeyFileExists = errors.New("key file already exists")

func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	alg := fs.String("alg", algEd25519, "Key algorithm: ed25519 | ecdsa | rsa")
	rsaBits := fs.Int("rsa-bits", rsaDefaultBits,
		fmt.Sprintf("RSA key size in bits, %d..%d (ignored for non-RSA)", rsaMinBits, rsaMaxBits))
	privOut := fs.String("priv-out", defaultPrivOut, "Path for the private key PEM file")
	pubOut := fs.String("pub-out", defaultPubOut, "Path for the public key PEM file")
	force := fs.Bool("force", false, "Overwrite existing output files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *privOut == "" || *pubOut == "" {
		return fmt.Errorf("-priv-out and -pub-out must be non-empty")
	}
	if *privOut == *pubOut {
		return fmt.Errorf("-priv-out and -pub-out must point to different files")
	}

	priv, err := generateKey(*alg, *rsaBits)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	if err := writeKeyPair(priv, *privOut, *pubOut, *force); err != nil {
		return fmt.Errorf("write key pair: %w", err)
	}

	fmt.Printf("Generated %s key pair:\n  private: %s (mode %#o)\n  public:  %s (mode %#o)\n",
		*alg, *privOut, privFileMode, *pubOut, pubFileMode)
	return nil
}

func generateKey(alg string, rsaBits int) (crypto.PrivateKey, error) {
	switch alg {
	case algEd25519:
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("ed25519: %w", err)
		}
		return priv, nil
	case algECDSA:
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("ecdsa P-256: %w", err)
		}
		return priv, nil
	case algRSA:
		if rsaBits < rsaMinBits || rsaBits > rsaMaxBits {
			return nil, fmt.Errorf("rsa-bits %d out of range [%d, %d]", rsaBits, rsaMinBits, rsaMaxBits)
		}
		priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
		if err != nil {
			return nil, fmt.Errorf("rsa %d-bit: %w", rsaBits, err)
		}
		return priv, nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q (supported: %s, %s, %s)",
			alg, algEd25519, algECDSA, algRSA)
	}
}

func encodePrivatePEM(priv crypto.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshal PKCS#8: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: pemTypePrivateKey, Bytes: der}), nil
}

func encodePublicPEM(priv crypto.PrivateKey) ([]byte, error) {
	signer, ok := priv.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("private key %T does not implement crypto.Signer", priv)
	}
	der, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil {
		return nil, fmt.Errorf("marshal PKIX: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: pemTypePublicKey, Bytes: der}), nil
}

// writeKeyPair encodes priv into PKCS#8 PEM and its public part into PKIX PEM,
// then writes both files. Without force, an existing destination returns
// errKeyFileExists wrapped with the path. If the public-key write fails after
// the private file has been created, the private file is removed best-effort
// so the caller doesn't see a half-finished state on disk.
func writeKeyPair(priv crypto.PrivateKey, privPath, pubPath string, force bool) error {
	privPEM, err := encodePrivatePEM(priv)
	if err != nil {
		return err
	}
	pubPEM, err := encodePublicPEM(priv)
	if err != nil {
		return err
	}

	if err := writeKeyFile(privPath, privPEM, privFileMode, force); err != nil {
		return fmt.Errorf("private key: %w", err)
	}
	if err := writeKeyFile(pubPath, pubPEM, pubFileMode, force); err != nil {
		// Roll back the private file so we never leave the user with a
		// private half of a key pair whose public half is unknown.
		_ = os.Remove(privPath)
		return fmt.Errorf("public key: %w", err)
	}
	return nil
}

// writeKeyFile writes data to path with mode. When force is false the call
// fails with [errKeyFileExists] if path already exists (O_EXCL); when true
// the existing content is truncated. On partial write failure the file is
// removed so the caller never sees a half-written key on disk.
func writeKeyFile(path string, data []byte, mode os.FileMode, force bool) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, mode)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s (use -force to overwrite)", errKeyFileExists, path)
		}
		return err
	}
	if _, werr := f.Write(data); werr != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return werr
	}
	if cerr := f.Close(); cerr != nil {
		_ = os.Remove(path)
		return cerr
	}
	return nil
}
