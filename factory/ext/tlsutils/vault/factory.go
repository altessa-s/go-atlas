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
// [WithConfiguredNames] restricts it to the configured names and
// [WithIssueOnBuild] obtains their certificates before Build returns; with
// either option the builder returns a wrapper around the go-tlsutils provider.
func Builder(client *vaultApi.Client, opts ...Option) tlsfactory.ProviderBuilder {
	settings := newOptions(opts...)

	return func(cfg *tlsconfig.Provider, env tlsfactory.ProviderEnv) (tlsproviders.Provider, error) {
		if client == nil {
			return nil, ErrClientRequired
		}

		v := cfg.Vault
		vaultOpts := []tlsvault.Option{
			tlsvault.WithVc(client),
			tlsvault.WithCommonName(v.CommonName),
			tlsvault.WithRole(v.Role),
			tlsvault.WithRenewBefore(v.RenewBefore),
		}
		vaultOpts = slices.AppendIf(vaultOpts, len(v.SubjectAlternativeNames) > 0, tlsvault.WithSubjectAlternativeNames(v.SubjectAlternativeNames...))
		vaultOpts = slices.AppendIf(vaultOpts, len(v.IPSubjectAlternativeNames) > 0, tlsvault.WithIPSubjectAlternativeNames(v.IPSubjectAlternativeNames...))
		vaultOpts = slices.AppendIf(vaultOpts, env.Logger != nil, tlsvault.WithLogger(env.Logger))
		vaultOpts = slices.AppendIfFunc(vaultOpts, env.CacheDir != "", func() []tlsvault.Option {
			return []tlsvault.Option{tlsvault.WithCacheDir(filepath.Join(env.CacheDir, "vault"))}
		})
		vaultOpts = slices.AppendIf(vaultOpts, env.OCSPStapler != nil, tlsvault.WithOcspStapler(env.OCSPStapler))

		p, err := tlsvault.New(vaultOpts...)
		if err != nil {
			return nil, err
		}
		if !settings.configuredNames && !settings.issueOnBuild {
			return p, nil
		}
		return newConfiguredNames(p, v, settings.configuredNames, settings.issueOnBuild), nil
	}
}
