// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/security/hmacsign"
)

// Verifier builds a [hmacsign.Verifier] from cfg: the scheme selected by
// cfg.Scheme, the primary cfg.Secret plus any cfg.AdditionalSecrets for
// rotation, and cfg.Tolerance as the replay window.
func Verifier(cfg *config.WebhookSignature) (*hmacsign.Verifier, error) {
	scheme, err := scheme(cfg)
	if err != nil {
		return nil, err
	}
	opts := []hmacsign.Option{hmacsign.WithTolerance(cfg.Tolerance)}
	if extra := additionalSecrets(cfg); len(extra) > 0 {
		opts = append(opts, hmacsign.WithSecrets(extra...))
	}
	return hmacsign.NewVerifier(scheme, []byte(cfg.Secret.Expose()), opts...), nil
}

// Signer builds a [hmacsign.Signer] from cfg. The signer always signs with the
// primary cfg.Secret; cfg.AdditionalSecrets (verifier-only) and cfg.Tolerance do
// not apply.
func Signer(cfg *config.WebhookSignature) (*hmacsign.Signer, error) {
	scheme, err := scheme(cfg)
	if err != nil {
		return nil, err
	}
	return hmacsign.NewSigner(scheme, []byte(cfg.Secret.Expose())), nil
}

// scheme resolves the config scheme string to a [hmacsign.Scheme].
func scheme(cfg *config.WebhookSignature) (hmacsign.Scheme, error) {
	if cfg == nil {
		return nil, fmt.Errorf("hmacsign/factory: configuration is required")
	}
	switch cfg.Scheme {
	case config.WebhookSchemeGitHub:
		return hmacsign.GitHub(), nil
	case config.WebhookSchemeStripe:
		return hmacsign.Stripe(), nil
	default:
		return nil, fmt.Errorf("hmacsign/factory: unknown webhook scheme %q", cfg.Scheme)
	}
}

// additionalSecrets exposes the rotation secrets as byte slices.
func additionalSecrets(cfg *config.WebhookSignature) [][]byte {
	if len(cfg.AdditionalSecrets) == 0 {
		return nil
	}
	out := make([][]byte, 0, len(cfg.AdditionalSecrets))
	for _, s := range cfg.AdditionalSecrets {
		out = append(out, []byte(s.Expose()))
	}
	return out
}
