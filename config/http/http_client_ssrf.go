// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	"fmt"
	"net/netip"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ClientSSRF is the YAML-driven SSRF (Server-Side Request Forgery)
// egress policy for transport/http/client. Translate it into client options
// via transport/http/client/factory.SSRFOptions and pass them to httpclient.New:
//
//	opts, err := factory.SSRFOptions(cfg.SSRF)
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
type ClientSSRF struct {
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

// DefaultClientSSRF returns the zero-value ClientSSRF, which keeps SSRF
// protection enabled with no CIDR exemptions — the strict, safe-by-default
// posture. It materializes no options because httpclient.New is already
// protected by default.
func DefaultClientSSRF() ClientSSRF {
	return ClientSSRF{}
}

// Validate performs validation on the ClientSSRF configuration. It verifies
// that every entry in AllowedCIDRs parses as a CIDR prefix. A nil receiver is
// valid and reports no error.
func (s *ClientSSRF) Validate() error {
	if s == nil {
		return nil
	}

	return validationconfig.ValidateStruct(s,
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
