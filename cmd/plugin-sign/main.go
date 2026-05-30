// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Command plugin-sign creates and verifies cryptographic signatures for Go plugins,
// and generates the key pairs used to do so. The CLI is verb-style.
//
// Usage:
//
//	plugin-sign keygen -alg ed25519 -priv-out private.pem -pub-out public.pem
//	plugin-sign sign   -key private.pem plugin1.so plugin2.so ...
//	plugin-sign sign   -key private.pem -dir ./plugins
//	plugin-sign verify -pubkey public.pem plugin.so
//
// The tool creates a .sig file for each plugin containing the raw signature bytes.
// Supports Ed25519 (recommended), ECDSA P-256, and RSA-PSS algorithms.
//
// Keys produced by `plugin-sign keygen` are PKCS#8 PEM for the private half and
// PKIX PEM for the public half — both directly consumable by `sign` / `verify`.
// External tools (e.g. openssl) that emit PKCS#8 / PKIX are also accepted.
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
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	// exitUsageError is the conventional exit code for "user passed bad arguments"
	// — distinct from the generic exit 1 used by log.Fatal for runtime failures.
	exitUsageError = 2

	// minArgvLenWithVerb is the minimum len(os.Args) when a verb is present:
	// argv[0] is the binary name, argv[1] is the verb.
	minArgvLenWithVerb = 2
)

func main() {
	if len(os.Args) < minArgvLenWithVerb {
		printUsage(os.Stderr)
		os.Exit(exitUsageError)
	}
	verb, args := os.Args[1], os.Args[2:]

	var err error
	switch verb {
	case "sign":
		err = runSign(args)
	case "verify":
		err = runVerify(args)
	case "keygen":
		err = runKeygen(args)
	case "-h", "-help", "--help", "help":
		printUsage(os.Stdout)
		return
	default:
		_, _ = fmt.Fprintf(os.Stderr, "unknown command %q\n\n", verb)
		printUsage(os.Stderr)
		os.Exit(exitUsageError)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func printUsage(w io.Writer) {
	const usageTemplate = `Usage: %[1]s <command> [options] [args...]

Commands:
  keygen   Generate a signing key pair (ed25519 | ecdsa | rsa)
  sign     Sign one or more plugin .so files
  verify   Verify .sig files for one or more plugins

Examples:
  %[1]s keygen -alg ed25519 -priv-out private.pem -pub-out public.pem
  %[1]s sign   -key private.pem plugin1.so plugin2.so
  %[1]s sign   -key private.pem -dir ./plugins
  %[1]s verify -pubkey public.pem plugin.so

Run '%[1]s <command> -h' to see flags for a specific command.
`
	_, _ = fmt.Fprintf(w, usageTemplate, os.Args[0])
}

func runSign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	keyFile := fs.String("key", "", "Path to private key PEM file (required)")
	dir := fs.String("dir", "", "Directory containing .so files to sign")
	verbose := fs.Bool("v", false, "Verbose output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *keyFile == "" {
		return fmt.Errorf("private key file required (-key)")
	}

	key, err := loadPrivateKey(*keyFile)
	if err != nil {
		return fmt.Errorf("load private key: %w", err)
	}

	files, err := collectPluginFiles(*dir, fs.Args())
	if err != nil {
		return err
	}

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

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	pubKey := fs.String("pubkey", "", "Path to public key PEM file (required)")
	dir := fs.String("dir", "", "Directory containing .so files to verify")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *pubKey == "" {
		return fmt.Errorf("public key file required (-pubkey)")
	}

	key, err := loadPublicKey(*pubKey)
	if err != nil {
		return fmt.Errorf("load public key: %w", err)
	}

	files, err := collectPluginFiles(*dir, fs.Args())
	if err != nil {
		return err
	}

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

func collectPluginFiles(dir string, positional []string) ([]string, error) {
	var files []string
	if dir != "" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read directory %q: %w", dir, err)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".so") {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
	}
	files = append(files, positional...)
	if len(files) == 0 {
		return nil, fmt.Errorf("no plugin files specified")
	}
	return files, nil
}

func signFile(path string, key crypto.PrivateKey) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	hash := sha256.Sum256(data)

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

	sigPath := path + ".sig"
	const defaultFileMode = 0o644
	if err := os.WriteFile(sigPath, signature, defaultFileMode); err != nil {
		return fmt.Errorf("write signature: %w", err)
	}

	return nil
}

func verifyFile(path string, key crypto.PublicKey) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	sigPath := path + ".sig"
	signature, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("read signature: %w", err)
	}

	hash := sha256.Sum256(data)

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

// loadPrivateKey reads a PEM-encoded private key. It accepts (in order)
// PKCS#8, PKCS#1 (legacy RSA), SEC1 EC, and a raw 32-byte Ed25519 seed. If
// none of those work the returned error joins the per-format failure so the
// caller can see which formats were tried and why each one rejected the
// input — e.g. an encrypted PKCS#8 PEM surfaces "x509: encrypted private
// key is not supported" instead of a generic parse error.
func loadPrivateKey(path string) (crypto.PrivateKey, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	var parseErrs []error

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key, nil
	} else {
		parseErrs = append(parseErrs, fmt.Errorf("PKCS#8: %w", err))
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	} else {
		parseErrs = append(parseErrs, fmt.Errorf("PKCS#1: %w", err))
	}

	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	} else {
		parseErrs = append(parseErrs, fmt.Errorf("SEC1 EC: %w", err))
	}

	if len(block.Bytes) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(block.Bytes), nil
	}
	parseErrs = append(parseErrs,
		fmt.Errorf("raw Ed25519 seed: block is %d bytes, expected %d", len(block.Bytes), ed25519.SeedSize))

	return nil, fmt.Errorf("failed to parse private key (PEM type %q): %w",
		block.Type, errors.Join(parseErrs...))
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
