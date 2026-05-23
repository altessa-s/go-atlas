// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// sigExtension is appended to the .so path to form the .sig file path.
const sigExtension = ".sig"

// SignatureMode controls how the manager handles plugin signatures.
type SignatureMode string

const (
	// SignatureDisabled disables signature verification.
	// SECURITY WARNING: This mode allows loading unsigned plugins which could
	// contain malicious code. Only use in development or when explicitly required.
	SignatureDisabled SignatureMode = "disabled"

	// SignatureRequire rejects plugins without a valid .sig file (default).
	// This is the recommended mode for production deployments.
	SignatureRequire SignatureMode = "require"

	// SignatureEnforce is an alias for [SignatureRequire].
	SignatureEnforce SignatureMode = "enforce"

	// SignatureWarn logs a warning but allows unsigned plugins. Plugins
	// with an invalid signature (bad .sig content) are still rejected.
	SignatureWarn SignatureMode = "warn"
)

// SignatureOptions configures plugin signature verification. Pass it to
// [WithSignature] when constructing the manager, or use
// [SignatureOptionsFromConfig] to convert from [config.PluginsSignature].
type SignatureOptions struct {
	// Mode controls verification behavior. Default is [SignatureRequire].
	Mode SignatureMode

	// PublicKey is the verification key. When set, PublicKeyPath is
	// ignored. Accepted types: [ed25519.PublicKey], [*ecdsa.PublicKey],
	// [*rsa.PublicKey].
	PublicKey crypto.PublicKey

	// PublicKeyPath is the path to a PEM-encoded public key file
	// (PKIX/SPKI "PUBLIC KEY" block). Used when PublicKey is nil.
	PublicKeyPath string
}

// signatureState is the resolved signature config stored in [options].
// A nil pubKey and empty pubKeys means verification is disabled.
type signatureState struct {
	mode    SignatureMode
	pubKey  crypto.PublicKey   // Single key (legacy)
	pubKeys []crypto.PublicKey // Multiple keys for rotation
}

// verifySignature checks a detached signature file against pluginData.
// Returns nil on success, [ErrSignatureInvalid] on mismatch.
func verifySignature(pubKey crypto.PublicKey, pluginData []byte, sigPath string) error {
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return err // caller distinguishes os.ErrNotExist
	}

	switch key := pubKey.(type) {
	case ed25519.PublicKey:
		digest := sha256.Sum256(pluginData)
		if !ed25519.Verify(key, digest[:], sig) {
			return ErrSignatureInvalid
		}
		return nil

	case *ecdsa.PublicKey:
		digest := sha256.Sum256(pluginData)
		if !ecdsa.VerifyASN1(key, digest[:], sig) {
			return ErrSignatureInvalid
		}
		return nil

	case *rsa.PublicKey:
		digest := sha256.Sum256(pluginData)
		if err := rsa.VerifyPSS(key, crypto.SHA256, digest[:], sig, nil); err != nil {
			return coreerrs.Wrapf(ErrSignatureInvalid, "RSA-PSS: %v", err)
		}
		return nil

	default:
		return fmt.Errorf("%w: unsupported key type %T", ErrSignatureConfig, pubKey)
	}
}

// parsePublicKeyPEM parses a PKIX-encoded PEM public key. Accepts
// Ed25519, ECDSA P-256, and RSA keys.
func parsePublicKeyPEM(pemData []byte) (crypto.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("%w: no PUBLIC KEY PEM block found", ErrSignatureConfig)
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, coreerrs.Wrapf(ErrSignatureConfig, "parse public key: %v", err)
	}

	switch k := key.(type) {
	case ed25519.PublicKey:
		return k, nil
	case *ecdsa.PublicKey:
		if k.Curve != elliptic.P256() {
			return nil, fmt.Errorf("%w: ECDSA curve %s not supported, use P-256",
				ErrSignatureConfig, k.Curve.Params().Name)
		}
		return k, nil
	case *rsa.PublicKey:
		return k, nil
	default:
		return nil, fmt.Errorf("%w: unsupported key type %T", ErrSignatureConfig, key)
	}
}

// loadPublicKey resolves the public key from [SignatureOptions].
// Returns (nil, nil) when verification is disabled.
func loadPublicKey(opts SignatureOptions) (crypto.PublicKey, error) {
	if opts.Mode == SignatureDisabled {
		return nil, nil //nolint:nilnil // nil means disabled
	}
	if opts.PublicKey != nil {
		return opts.PublicKey, nil
	}
	if opts.PublicKeyPath == "" {
		return nil, fmt.Errorf("%w: mode=%q but no public key configured",
			ErrSignatureConfig, opts.Mode)
	}
	data, err := os.ReadFile(opts.PublicKeyPath)
	if err != nil {
		return nil, coreerrs.Wrapf(ErrSignatureConfig, "read key file %q: %v",
			opts.PublicKeyPath, err)
	}
	return parsePublicKeyPEM(data)
}

// verifyPluginSignature checks the detached .sig file when signature
// verification is enabled. On failure it quarantines the plugin.
// Returns nil when verification is disabled or succeeds.
func (m *Manager) verifyPluginSignature(filename, path string, data []byte, fileHash string) error {
	// Track signature verification attempt
	if m.metrics != nil {
		m.metrics.signatureAttempts.Inc()
		stop := m.metrics.signatureDuration.Start()
		defer stop()
	}

	sig := m.opts.signature

	// If signature verification is disabled, allow unsigned plugins.
	if sig.mode == SignatureDisabled {
		if m.metrics != nil {
			m.metrics.signatureSuccess.Inc()
		}
		return nil
	}

	// If no public key is configured but signature is required, fail.
	if sig.pubKey == nil && len(sig.pubKeys) == 0 {
		if sig.mode == SignatureRequire || sig.mode == SignatureEnforce {
			if m.metrics != nil {
				m.metrics.signatureFailures.Inc()
			}
			m.addQuarantine(filename, fileHash)
			return coreerrs.Wrapf(ErrSignatureConfig, "plugin %q: signature required but no public key configured", filename)
		}
		// SignatureWarn or empty mode with no key - allow unsigned.
		if m.metrics != nil {
			m.metrics.signatureSuccess.Inc()
		}
		return nil
	}

	sigPath := path + sigExtension

	// Use multi-key verification if multiple keys configured
	var err error
	if len(sig.pubKeys) > 0 {
		err = verifySignatureMultiKey(sig.pubKeys, data, sigPath)
	} else {
		err = verifySignature(sig.pubKey, data, sigPath)
	}
	if err == nil {
		if m.metrics != nil {
			m.metrics.signatureSuccess.Inc()
		}
		m.logger.Info("plugin signature verified",
			slog.String("file", filename),
			slog.String("hash", fileHash[:min(hashPrefixLen, len(fileHash))]),
		)
		return nil
	}

	// Missing .sig — behavior depends on mode.
	if errors.Is(err, os.ErrNotExist) {
		if sig.mode == SignatureWarn {
			m.logger.Warn("plugin signature file missing; loading anyway (mode=warn)",
				slog.String("file", filename),
			)
			if m.metrics != nil {
				m.metrics.signatureSuccess.Inc() // Allowed in warn mode
			}
			return nil
		}
		// require / enforce
		if m.metrics != nil {
			m.metrics.signatureFailures.Inc()
		}
		m.addQuarantine(filename, fileHash)
		return coreerrs.Wrapf(ErrSignatureMissing, "plugin %q: %s not found", filename, sigPath)
	}

	// Bad signature or read error — quarantine unconditionally.
	if m.metrics != nil {
		m.metrics.signatureFailures.Inc()
	}
	m.addQuarantine(filename, fileHash)
	return coreerrs.Wrapf(ErrSignatureInvalid, "plugin %q", filename)
}

// SignatureOptionsFromConfig converts a [config.PluginsSignature] to
// runtime [SignatureOptions].
func SignatureOptionsFromConfig(mode, publicKeyPath string) SignatureOptions {
	return SignatureOptions{
		Mode:          SignatureMode(mode),
		PublicKeyPath: publicKeyPath,
	}
}
