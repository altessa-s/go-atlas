// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factoryconv

import (
	"crypto/tls"
	"fmt"

	"github.com/altessa-s/go-tlsutils"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
)

// ServerTLS obtains the configured provider certificate and applies the minimum TLS version.
func ServerTLS(b *corefactory.Base, providers *tlsproviders.Providers, cfg *tlsconfig.Server) (*tls.Config, error) {
	if err := b.RequireDependency(providers, "tls providers"); err != nil {
		return nil, err
	}

	providerType := tlsproviders.ProviderType(cfg.Provider)
	provider, ok := providers.Get(providerType)
	if !ok {
		return nil, fmt.Errorf("tls provider %q not configured", cfg.Provider)
	}

	tlsConfig, err := provider.TLSConfig()
	if err != nil {
		return nil, b.WrapError(err, "failed to create TLS config from provider")
	}

	if err := tlsutils.ApplyMinTLSVersion(tlsConfig, cfg.MinTLSVersion); err != nil {
		return nil, b.WrapError(err, "tls.minVersion")
	}

	return tlsConfig, nil
}
