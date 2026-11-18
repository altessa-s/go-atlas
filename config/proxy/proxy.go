// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxyconfig

import (
	"net/url"

	"github.com/altessa-s/go-atlas/config/internal/cfgerrors"

	netproxy "github.com/altessa-s/go-atlas-net/proxyconfig"
)

// Config uses the shared outbound proxy schema with Atlas validation errors.
type Config netproxy.Config

// Default preserves the underlying transport's proxy policy.
func Default() Config { return Config{} }

// Validate checks that the selected proxy can be resolved.
func (p *Config) Validate() error {
	_, err := p.URLValue()
	return err
}

// URLValue resolves an explicit proxy. Empty mode and none both yield a nil URL.
func (p *Config) URLValue() (*url.URL, error) {
	u, err := (*netproxy.Config)(p).URLValue()
	if err != nil {
		return nil, cfgerrors.Invalidf("proxy: %w", err)
	}
	return u, nil
}
