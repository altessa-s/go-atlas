// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	"github.com/altessa-s/go-atlas-net/egressrule"
	"github.com/altessa-s/go-atlas/config/internal/validators"

	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ClientSSRF is the YAML-driven SSRF (Server-Side Request Forgery)
// egress policy for go-atlas-net/http/client. Translate it into client options
// via factory/ext/net/httpclient.SSRFOptions and pass them to httpclient.New:
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
//
// Allow and Deny turn the built-in check into an allow/deny egress policy
// (go-atlas-net/http/client.EgressPolicy): a destination a deny rule matches is
// refused (deny wins), else one an allow rule matches is allowed, else a
// private or reserved address is refused. AllowedCIDRs are allow rules of
// that policy, one per prefix, after the Allow rules and named
// "allowedCIDRs[i]" in decisions.
type ClientSSRF struct {
	// Disabled turns SSRF protection off. The zero value (false) keeps
	// protection ENABLED — see the type doc for the safe-by-default rationale.
	// Set to true only for clients that must reach arbitrary private addresses
	// and cannot enumerate them via AllowedCIDRs.
	Disabled bool `yaml:"disabled"`

	// AllowedCIDRs lists CIDR prefixes exempted from SSRF blocking, for known
	// internal service-to-service targets on private networks (e.g.
	// "10.0.1.0/24"). Ignored when Disabled is true.
	AllowedCIDRs []string `yaml:"allowedCIDRs"`

	// Allow lists rules that exempt resolved addresses from the private
	// denial; each one needs a CIDR, and its host and port narrow it.
	// Ignored when Disabled is true.
	Allow []egressrule.Rule `yaml:"allow"`

	// Deny lists rules that refuse a destination whatever else matches; each
	// one needs at least one matcher. A rule without a CIDR also refuses a
	// matching host before it is resolved. Ignored when Disabled is true.
	Deny []egressrule.Rule `yaml:"deny"`
}

// DefaultClientSSRF returns the zero-value ClientSSRF, which keeps SSRF
// protection enabled with no CIDR exemptions — the strict, safe-by-default
// posture. It materializes no options because httpclient.New is already
// protected by default.
func DefaultClientSSRF() ClientSSRF {
	return ClientSSRF{}
}

// Validate performs validation on the ClientSSRF configuration. It verifies
// that every entry in AllowedCIDRs parses as a CIDR prefix and that the Allow
// and Deny rules compile ([egressrule.Rule.Parse]: the checks of the egress
// engine, error naming the rule, e.g. "deny[1].host"). A nil receiver is
// valid and reports no error.
func (s *ClientSSRF) Validate() error {
	if s == nil {
		return nil
	}

	return validationconfig.ValidateStruct(s,
		validation.Field(&s.AllowedCIDRs, validation.Each(validators.CIDRPrefix)),
		validation.Field(&s.Allow, validation.By(validateEgressRules(egressrule.Allow))),
		validation.Field(&s.Deny, validation.By(validateEgressRules(egressrule.Deny))),
	)
}

// validateEgressRules returns the validation of the rules of list.
func validateEgressRules(list egressrule.List) validation.RuleFunc {
	return func(value any) error {
		rules, _ := value.([]egressrule.Rule)
		for i, r := range rules {
			if _, err := r.Parse(list, i); err != nil {
				return err
			}
		}
		return nil
	}
}
