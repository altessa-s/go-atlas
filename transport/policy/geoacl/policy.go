// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"context"
	"net/netip"
	"slices"

	"github.com/altessa-s/go-atlas/transport/policy/endpointrule"
)

// GeoInfo holds geographic location data resolved from an IP address.
type GeoInfo struct {
	ContinentCode string // "EU", "NA", "AS", "AF", "OC", "SA", "AN"
	CountryCode   string // ISO 3166-1 alpha-2: "US", "DE", "RU"
	RegionCode    string // subdivision: "CA", "BY"
}

// FullRegion returns the combined country-region code (e.g. "US-CA").
func (g GeoInfo) FullRegion() string {
	if g.CountryCode == "" || g.RegionCode == "" {
		return ""
	}
	return g.CountryCode + "-" + g.RegionCode
}

// GeoResolver resolves an IP address to geographic information.
type GeoResolver interface {
	Resolve(ctx context.Context, ip netip.Addr) (GeoInfo, error)
}

// Policy determines the default action when a location matches neither the
// allow lists nor the deny lists of a rule.
type Policy int

const (
	// PolicyDeny denies access by default (allowlist mode).
	PolicyDeny Policy = iota

	// PolicyAllow allows access by default (denylist mode).
	PolicyAllow
)

// AccessRule defines geographic allow and deny lists for an endpoint or
// group of endpoints.
type AccessRule struct {
	AllowContinents []string // ["EU", "NA"]
	DenyContinents  []string // ["AS"]
	AllowCountries  []string // ["US", "CA"]
	DenyCountries   []string // ["RU", "CN"]
	AllowRegions    []string // ["US-CA", "DE-BY"]
	DenyRegions     []string // ["US-TX"]
}

// Registry holds a collection of geographic access rules keyed by endpoint
// name or regex pattern, plus an optional default rule and a fallback policy.
//
// Rule registration and lookup (Register, RegisterPattern, RegisterEndpoints,
// SetDefault, Lookup) are provided by the embedded [endpointrule.Registry].
type Registry struct {
	endpointrule.Registry[AccessRule]

	policy Policy
}

// NewRegistry creates a new Registry with the given default policy.
func NewRegistry(policy Policy) *Registry {
	return &Registry{policy: policy}
}

// Evaluate checks whether the given IP address is allowed to access
// the given endpoint based on geographic location.
//
// The resolver is called to determine the geographic location of the IP.
// If the resolver returns an error, it is propagated to the caller.
//
// Decision logic within a matched rule (all deny checks run before allow checks,
// so a continent-level deny overrides a country-level allow):
//  1. Region in DenyRegions     → deny
//  2. Country in DenyCountries  → deny
//  3. Continent in DenyContinents → deny
//  4. Region in AllowRegions    → allow
//  5. Country in AllowCountries → allow
//  6. Continent in AllowContinents → allow
//  7. Neither, and the rule has an allow list → deny
//  8. Neither, and the rule has no allow list → fall back to Registry.policy
//
// A rule with a non-empty allow list is an allowlist for its endpoint whatever
// the registry policy is. A location the resolver could not determine (neither
// country nor continent, which is what a GeoIP database returns for loopback
// and private addresses) is on no list: an allow list denies it, a rule with
// deny lists only leaves it to the registry policy.
//
// If no rule matches the endpoint, the registry-level policy applies directly.
func (r *Registry) Evaluate(ctx context.Context, resolver GeoResolver, ip netip.Addr, endpoint string) (bool, error) {
	rule, found := r.Lookup(endpoint)
	if !found {
		return r.policy == PolicyAllow, nil
	}
	if rule == nil {
		return false, nil
	}

	geo, err := resolver.Resolve(ctx, ip)
	if err != nil {
		return false, err
	}

	return evaluateRule(rule, geo, r.policy), nil
}

func evaluateRule(rule *AccessRule, geo GeoInfo, policy Policy) bool {
	hasAllow := len(rule.AllowRegions)+len(rule.AllowCountries)+len(rule.AllowContinents) > 0

	fullRegion := geo.FullRegion()

	// Deny checks (most specific first).
	if fullRegion != "" && slices.Contains(rule.DenyRegions, fullRegion) {
		return false
	}
	if geo.CountryCode != "" && slices.Contains(rule.DenyCountries, geo.CountryCode) {
		return false
	}
	if geo.ContinentCode != "" && slices.Contains(rule.DenyContinents, geo.ContinentCode) {
		return false
	}

	// Allow checks (most specific first).
	if fullRegion != "" && slices.Contains(rule.AllowRegions, fullRegion) {
		return true
	}
	if geo.CountryCode != "" && slices.Contains(rule.AllowCountries, geo.CountryCode) {
		return true
	}
	if geo.ContinentCode != "" && slices.Contains(rule.AllowContinents, geo.ContinentCode) {
		return true
	}

	// An allow list makes the rule an allowlist for its endpoint: a location
	// outside it, an unknown one included, is denied even when the registry
	// default is PolicyAllow.
	if hasAllow {
		return false
	}

	return policy == PolicyAllow
}
