// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"net/netip"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
)

// SSRFOptions materializes the SSRF policy into a slice of
// net/http/client options ready to be passed to httpclient.New.
//
// Because httpclient.New enables SSRF protection by default, a nil receiver or
// an enabled policy with no AllowedCIDRs and no rules returns (nil, nil) — the
// client keeps its protected default. A Disabled policy returns
// httpclient.WithoutSSRFProtection and ignores the rest. Allow and Deny rules
// are compiled with httpclient.ParseEgressPolicy and installed with
// httpclient.WithEgressPolicy; AllowedCIDRs return
// httpclient.WithSSRFAllowedCIDRs, which with a policy adds one allow rule per
// prefix after the Allow rules (named "allowedCIDRs[i]"). An unparseable CIDR
// or a rule the egress engine rejects surfaces as an error here rather than
// at request time; rule errors wrap httpclient.ErrInvalidEgressRule.
func SSRFOptions(s *httpconfig.ClientSSRF) ([]httpclient.Option, error) {
	if s == nil {
		return nil, nil
	}

	if s.Disabled {
		return []httpclient.Option{httpclient.WithoutSSRFProtection()}, nil
	}

	var opts []httpclient.Option
	if len(s.Allow) > 0 || len(s.Deny) > 0 {
		policy, err := httpclient.ParseEgressPolicy(s.Allow, s.Deny)
		if err != nil {
			return nil, fmt.Errorf("HTTPClientSSRF: %w", err)
		}
		opts = append(opts, httpclient.WithEgressPolicy(policy))
	}

	if len(s.AllowedCIDRs) == 0 {
		return opts, nil
	}

	prefixes := make([]netip.Prefix, 0, len(s.AllowedCIDRs))
	for _, cidr := range s.AllowedCIDRs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("HTTPClientSSRF: parse allowed CIDR %q: %w", cidr, err)
		}
		prefixes = append(prefixes, prefix)
	}

	return append(opts, httpclient.WithSSRFAllowedCIDRs(prefixes...)), nil
}
