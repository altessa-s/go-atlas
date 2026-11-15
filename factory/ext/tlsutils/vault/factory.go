// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"path/filepath"

	"github.com/altessa-s/go-atlas-core/collections/slices"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	tlsfactory "github.com/altessa-s/go-atlas/factory/ext/tlsutils"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
	tlsvault "github.com/altessa-s/go-tlsutils/providers/vault"
	vaultApi "github.com/hashicorp/vault/api"
)

// ErrClientRequired is returned when the Vault TLS provider is built without
// a Vault client.
var ErrClientRequired = errors.New("vault client is required")

// Builder returns the provider builder of the vault type, ready for
// tlsfactory.ProvidersBuilder.UseProvider. The provider issues certificates
// from the PKI engine through client, configured by the vault section, and
// caches them under the vault subdirectory of the cache directory.
func Builder(client *vaultApi.Client) tlsfactory.ProviderBuilder {
	return func(cfg *tlsconfig.Provider, env tlsfactory.ProviderEnv) (tlsproviders.Provider, error) {
		if client == nil {
			return nil, ErrClientRequired
		}

		v := cfg.Vault
		opts := []tlsvault.Option{
			tlsvault.WithVc(client),
			tlsvault.WithCommonName(v.CommonName),
			tlsvault.WithRole(v.Role),
			tlsvault.WithRenewBefore(v.RenewBefore),
		}
		opts = slices.AppendIf(opts, len(v.SubjectAlternativeNames) > 0, tlsvault.WithSubjectAlternativeNames(v.SubjectAlternativeNames...))
		opts = slices.AppendIf(opts, len(v.IPSubjectAlternativeNames) > 0, tlsvault.WithIPSubjectAlternativeNames(v.IPSubjectAlternativeNames...))
		opts = slices.AppendIf(opts, env.Logger != nil, tlsvault.WithLogger(env.Logger))
		opts = slices.AppendIfFunc(opts, env.CacheDir != "", func() []tlsvault.Option {
			return []tlsvault.Option{tlsvault.WithCacheDir(filepath.Join(env.CacheDir, "vault"))}
		})
		opts = slices.AppendIf(opts, env.OCSPStapler != nil, tlsvault.WithOcspStapler(env.OCSPStapler))

		p, err := tlsvault.New(opts...)
		if err != nil {
			return nil, err
		}
		return p, nil
	}
}
