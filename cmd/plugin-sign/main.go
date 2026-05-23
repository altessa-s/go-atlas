// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Command plugin-sign creates cryptographic signatures for Go plugins.
//
// Usage:
//
//	plugin-sign -key private.pem plugin1.so plugin2.so ...
//	plugin-sign -key private.pem -dir ./plugins
//
// The tool creates a .sig file for each plugin containing the raw signature bytes.
// Supports Ed25519 (recommended), ECDSA P-256, and RSA-PSS algorithms.
//
// Generate key pairs:
//
//	# Ed25519 (recommended)
//	openssl genpkey -algorithm ed25519 -out private.pem
//	openssl pkey -in private.pem -pubout -out public.pem
//
//	# ECDSA P-256
//	openssl ecparam -name prime256v1 -genkey -out private.pem
//	openssl ec -in private.pem -pubout -out public.pem
//
//	# RSA-PSS
//	openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out private.pem
//	openssl rsa -in private.pem -pubout -out public.pem
package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	keyFile = flag.String("key", "", "Path to private key PEM file (required)")
	dir     = flag.String("dir", "", "Directory containing .so files to sign")
	verify  = flag.Bool("verify", false, "Verify signatures instead of creating them")
	pubKey  = flag.String("pubkey", "", "Path to public key PEM file (for verification)")
	verbose = flag.Bool("v", false, "Verbose output")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [plugin.so ...]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nSigns Go plugin files with cryptographic signatures.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  Sign specific plugins:\n")
		fmt.Fprintf(os.Stderr, "    %s -key private.pem plugin1.so plugin2.so\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n  Sign all plugins in a directory:\n")
		fmt.Fprintf(os.Stderr, "    %s -key private.pem -dir ./plugins\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n  Verify signatures:\n")
		fmt.Fprintf(os.Stderr, "    %s -verify -pubkey public.pem plugin.so\n", os.Args[0])
	}
	flag.Parse()

	if *verify {
		if err := runVerify(); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := runSign(); err != nil {
		log.Fatal(err)
	}
}

func runSign() error {
	if *keyFile == "" {
		return fmt.Errorf("private key file required (-key)")
	}

	// Load private key
	key, err := loadPrivateKey(*keyFile)
	if err != nil {
		return fmt.Errorf("load private key: %w", err)
	}

	// Collect .so files to sign
	var files []string
	if *dir != "" {
		entries, err := os.ReadDir(*dir)
		if err != nil {
			return fmt.Errorf("read directory %q: %w", *dir, err)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".so") {
				files = append(files, filepath.Join(*dir, e.Name()))
			}
		}
	}
	files = append(files, flag.Args()...)

	if len(files) == 0 {
		return fmt.Errorf("no plugin files specified")
	}

	// Sign each file
	for _, file := range files {
		if err := signFile(file, key); err != nil {
			return fmt.Errorf("sign %q: %w", file, err)
		}
		if *verbose {
			fmt.Printf("Signed: %s\n", file)
		}
	}

	fmt.Printf("Successfully signed %d plugin(s)\n", len(files))
	return nil
}

func runVerify() error {
	if *pubKey == "" {
		return fmt.Errorf("public key file required (-pubkey) for verification")
	}

	// Load public key
	key, err := loadPublicKey(*pubKey)
	if err != nil {
		return fmt.Errorf("load public key: %w", err)
	}

	// Collect .so files to verify
	var files []string
	if *dir != "" {
		entries, err := os.ReadDir(*dir)
		if err != nil {
			return fmt.Errorf("read directory %q: %w", *dir, err)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".so") {
				files = append(files, filepath.Join(*dir, e.Name()))
			}
		}
	}
	files = append(files, flag.Args()...)

	if len(files) == 0 {
		return fmt.Errorf("no plugin files specified")
	}

	// Verify each file
	var failed int
	for _, file := range files {
		if err := verifyFile(file, key); err != nil {
			fmt.Printf("✗ %s: %v\n", file, err)
			failed++
		} else {
			fmt.Printf("✓ %s: signature valid\n", file)
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d signatures invalid", failed, len(files))
	}
	return nil
}

func signFile(path string, key crypto.PrivateKey) error {
	// Read plugin file
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Compute hash
	hash := sha256.Sum256(data)

	// Sign based on key type
	var signature []byte
	switch k := key.(type) {
	case ed25519.PrivateKey:
		signature = ed25519.Sign(k, hash[:])
	case *ecdsa.PrivateKey:
		signature, err = ecdsa.SignASN1(rand.Reader, k, hash[:])
		if err != nil {
			return fmt.Errorf("ECDSA sign: %w", err)
		}
	case *rsa.PrivateKey:
		signature, err = rsa.SignPSS(rand.Reader, k, crypto.SHA256, hash[:], nil)
		if err != nil {
			return fmt.Errorf("RSA-PSS sign: %w", err)
		}
	default:
		return fmt.Errorf("unsupported key type: %T", key)
	}

	// Write signature file
	sigPath := path + ".sig"
	if err := os.WriteFile(sigPath, signature, 0o644); err != nil {
		return fmt.Errorf("write signature: %w", err)
	}

	return nil
}

func verifyFile(path string, key crypto.PublicKey) error {
	// Read plugin file
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Read signature file
	sigPath := path + ".sig"
	signature, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("read signature: %w", err)
	}

	// Compute hash
	hash := sha256.Sum256(data)

	// Verify based on key type
	switch k := key.(type) {
	case ed25519.PublicKey:
		if !ed25519.Verify(k, hash[:], signature) {
			return fmt.Errorf("Ed25519 signature verification failed")
		}
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(k, hash[:], signature) {
			return fmt.Errorf("ECDSA signature verification failed")
		}
	case *rsa.PublicKey:
		if err := rsa.VerifyPSS(k, crypto.SHA256, hash[:], signature, nil); err != nil {
			return fmt.Errorf("RSA-PSS signature verification failed: %w", err)
		}
	default:
		return fmt.Errorf("unsupported key type: %T", key)
	}

	return nil
}

func loadPrivateKey(path string) (crypto.PrivateKey, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	// Try parsing as PKCS8 (works for all key types)
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Try parsing as PKCS1 (RSA only, for legacy keys)
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Try parsing as EC private key
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Try raw Ed25519 seed (32 bytes)
	if len(block.Bytes) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(block.Bytes), nil
	}

	return nil, fmt.Errorf("failed to parse private key")
}

func loadPublicKey(path string) (crypto.PublicKey, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	return pub, nil
}

// copyFile copies a file from src to dst.
func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
