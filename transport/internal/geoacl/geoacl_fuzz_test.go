// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/geoacl"
)

// staticResolver answers with one location, so a target exercises the decision
// table rather than a geo database.
type staticResolver struct {
	geo geoacl.GeoInfo
	err error
}

func (r staticResolver) Resolve(context.Context, netip.Addr) (geoacl.GeoInfo, error) {
	return r.geo, r.err
}

// FuzzEvaluateFollowsTheDocumentedPrecedence restates
// [geoacl.Registry.Evaluate]'s six-step decision independently and requires the
// implementation to agree for every combination of location and lists.
//
// Six steps in a fixed order — region, country, continent, denies before allows
// — is more precedence than anyone holds in their head, and the same location
// can legitimately match at three levels at once. That is where an ordering
// mistake hides: a country-level allow that quietly overrides a region-level
// deny looks correct in every test written from one example.
func FuzzEvaluateFollowsTheDocumentedPrecedence(f *testing.F) {
	f.Add("EU", "DE", "BY", true, false, false, false, false, false, true)
	f.Add("EU", "DE", "BY", false, true, false, false, false, false, false)
	f.Add("NA", "US", "CA", false, false, true, true, true, true, false)
	f.Add("", "", "", false, false, false, false, false, false, true)
	f.Add("AS", "", "XX", true, true, true, true, true, true, false)

	f.Fuzz(func(
		t *testing.T,
		continent, country, region string,
		denyRegion, denyCountry, denyContinent bool,
		allowRegion, allowCountry, allowContinent bool,
		defaultAllow bool,
	) {
		geo := geoacl.GeoInfo{ContinentCode: continent, CountryCode: country, RegionCode: region}
		fullRegion := geo.FullRegion()

		rule := &geoacl.AccessRule{}
		// Every list is populated from the location itself, so each flag decides
		// whether this exact location matches at that level — which is what puts
		// the ordering under pressure instead of the matching.
		if denyRegion && fullRegion != "" {
			rule.DenyRegions = []string{fullRegion}
		}
		if denyCountry && country != "" {
			rule.DenyCountries = []string{country}
		}
		if denyContinent && continent != "" {
			rule.DenyContinents = []string{continent}
		}
		if allowRegion && fullRegion != "" {
			rule.AllowRegions = []string{fullRegion}
		}
		if allowCountry && country != "" {
			rule.AllowCountries = []string{country}
		}
		if allowContinent && continent != "" {
			rule.AllowContinents = []string{continent}
		}

		policy := geoacl.PolicyDeny
		if defaultAllow {
			policy = geoacl.PolicyAllow
		}

		registry := geoacl.NewRegistry(policy)
		registry.Register("/api", rule)

		// The documented order, spelled out here rather than reused from the
		// implementation: an oracle that shares code with its subject agrees
		// with it even when both are wrong.
		want := defaultAllow
		switch {
		case denyRegion && fullRegion != "":
			want = false
		case denyCountry && country != "":
			want = false
		case denyContinent && continent != "":
			want = false
		case allowRegion && fullRegion != "":
			want = true
		case allowCountry && country != "":
			want = true
		case allowContinent && continent != "":
			want = true
		}

		got, err := registry.Evaluate(t.Context(), staticResolver{geo: geo}, netip.MustParseAddr("10.0.0.1"), "/api")
		require.NoError(t, err)
		require.Equal(t, want, got, "geo=%+v rule=%+v policy=%v", geo, rule, policy)
	})
}

// FuzzEvaluateFailsClosedOnResolverError pins what happens when the geo lookup
// itself fails: the request is refused, whatever the default policy says.
//
// A resolver is a network dependency, so it fails routinely — and a geo ACL
// that falls back to "allow" when it cannot tell where a request came from
// stops being an ACL exactly when it is under load or under attack.
func FuzzEvaluateFailsClosedOnResolverError(f *testing.F) {
	f.Add("lookup failed", true, "/api")
	f.Add("", false, "/api")
	f.Add("timeout", true, "/unregistered")

	f.Fuzz(func(t *testing.T, message string, defaultAllow bool, endpoint string) {
		policy := geoacl.PolicyDeny
		if defaultAllow {
			policy = geoacl.PolicyAllow
		}

		registry := geoacl.NewRegistry(policy)
		registry.Register("/api", &geoacl.AccessRule{AllowCountries: []string{"DE"}})

		resolverErr := errors.New(message)
		allowed, err := registry.Evaluate(t.Context(),
			staticResolver{err: resolverErr}, netip.MustParseAddr("10.0.0.1"), endpoint)

		if endpoint != "/api" {
			// No rule matched, so the resolver is never consulted and the
			// registry policy decides on its own.
			require.NoError(t, err)
			require.Equal(t, defaultAllow, allowed)
			return
		}

		require.ErrorIs(t, err, resolverErr, "the resolver failure must reach the caller")
		require.False(t, allowed, "a request whose location is unknown must not be allowed")
	})
}
