// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"
	"net/netip"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// HTTPClientSSRF is the YAML-driven SSRF (Server-Side Request Forgery)
// egress policy for transport/http/client. Translate it into client options
// via ClientOptions and pass them to httpclient.New:
//
//	opts, err := cfg.SSRF.ClientOptions()
//	if err != nil {
//	    return err
//	}
//	client := httpclient.New(opts...)
//
// Safe-by-default: the zero value keeps SSRF protection ENABLED (matching the
// httpclient.New default), so a config that omits the block still blocks
// connections to private and local IP addresses (RFC1918, loopback,
// link-local). Set Disabled to opt a config-driven client out, or list
// AllowedCIDRs to exempt known internal networks while keeping protection on.
type HTTPClientSSRF struct {
	// Disabled turns SSRF protection off. The zero value (false) keeps
	// protection ENABLED — see the type doc for the safe-by-default rationale.
	// Set to true only for clients that must reach arbitrary private addresses
	// and cannot enumerate them via AllowedCIDRs.
	Disabled bool `yaml:"disabled"`

	// AllowedCIDRs lists CIDR prefixes exempted from SSRF blocking, for known
	// internal service-to-service targets on private networks (e.g.
	// "10.0.1.0/24"). Ignored when Disabled is true.
	AllowedCIDRs []string `yaml:"allowed_cidrs"`
}

// DefaultHTTPClientSSRF returns the zero-value HTTPClientSSRF, which keeps SSRF
// protection enabled with no CIDR exemptions — the strict, safe-by-default
// posture. It materializes no options because httpclient.New is already
// protected by default.
func DefaultHTTPClientSSRF() HTTPClientSSRF {
	return HTTPClientSSRF{}
}

// Validate performs validation on the HTTPClientSSRF configuration. It verifies
// that every entry in AllowedCIDRs parses as a CIDR prefix. A nil receiver is
// valid and reports no error.
func (s *HTTPClientSSRF) Validate() error {
	if s == nil {
		return nil
	}

	return ValidateStruct(s,
		validation.Field(&s.AllowedCIDRs, validation.Each(validation.By(validateCIDRPrefix))),
	)
}

// validateCIDRPrefix reports whether value is a parseable CIDR prefix.
func validateCIDRPrefix(value any) error {
	raw, _ := value.(string)
	if _, err := netip.ParsePrefix(raw); err != nil {
		return fmt.Errorf("must be a valid CIDR prefix (e.g. 10.0.0.0/8): %w", err)
	}
	return nil
}

// ClientOptions materializes the SSRF policy into a slice of
// transport/http/client options ready to be passed to httpclient.New.
//
// Because httpclient.New enables SSRF protection by default, a nil receiver or
// an enabled policy with no AllowedCIDRs returns (nil, nil) — the client keeps
// its protected default. A Disabled policy returns httpclient.WithoutSSRFProtection.
// An enabled policy with AllowedCIDRs returns httpclient.WithSSRFAllowedCIDRs.
// An unparseable CIDR surfaces as an error here rather than at request time.
func (s *HTTPClientSSRF) ClientOptions() ([]httpclient.Option, error) {
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
