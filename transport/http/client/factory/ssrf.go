// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"net/netip"

	"github.com/altessa-s/go-atlas/config"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// SSRFOptions materializes the SSRF policy into a slice of
// transport/http/client options ready to be passed to httpclient.New.
//
// Because httpclient.New enables SSRF protection by default, a nil receiver or
// an enabled policy with no AllowedCIDRs returns (nil, nil) — the client keeps
// its protected default. A Disabled policy returns httpclient.WithoutSSRFProtection.
// An enabled policy with AllowedCIDRs returns httpclient.WithSSRFAllowedCIDRs.
// An unparseable CIDR surfaces as an error here rather than at request time.
func SSRFOptions(s *config.HTTPClientSSRF) ([]httpclient.Option, error) {
	if s == nil {
		return nil, nil
	}

	if s.Disabled {
		return []httpclient.Option{httpclient.WithoutSSRFProtection()}, nil
	}

	if len(s.AllowedCIDRs) == 0 {
		return nil, nil
	}

	prefixes := make([]netip.Prefix, 0, len(s.AllowedCIDRs))
	for _, cidr := range s.AllowedCIDRs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("HTTPClientSSRF: parse allowed CIDR %q: %w", cidr, err)
		}
		prefixes = append(prefixes, prefix)
	}

	return []httpclient.Option{httpclient.WithSSRFAllowedCIDRs(prefixes...)}, nil
}
