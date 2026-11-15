// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// A rule with an allow list restricts its endpoint under either registry
// policy: PolicyAllow used to let every location outside the list through.
func TestEvaluate_AllowListIsDefaultDenyUnderPolicyAllow(t *testing.T) {
	t.Parallel()
	const endpoint = "/admin.AdminService/Delete"

	rules := map[string]*AccessRule{
		"countries":  {AllowCountries: []string{"US"}},
		"continents": {AllowContinents: []string{"NA"}},
		"regions":    {AllowRegions: []string{"US-CA"}},
	}
	inside := GeoInfo{ContinentCode: "NA", CountryCode: "US", RegionCode: "CA"}
	outside := GeoInfo{ContinentCode: "EU", CountryCode: "DE", RegionCode: "BY"}

	for name, rule := range rules {
		for _, policy := range []Policy{PolicyAllow, PolicyDeny} {
			t.Run(fmt.Sprintf("%s/policy=%d", name, policy), func(t *testing.T) {
				t.Parallel()
				reg := NewRegistry(policy)
				reg.Register(endpoint, rule)

				got, err := reg.Evaluate(t.Context(), &mockResolver{geo: inside}, testIP, endpoint)
				require.NoError(t, err)
				require.True(t, got, "a listed location is allowed")

				got, err = reg.Evaluate(t.Context(), &mockResolver{geo: outside}, testIP, endpoint)
				require.NoError(t, err)
				require.False(t, got, "a location outside the allow list is denied")
			})
		}
	}
}

func TestEvaluate_UnknownLocation(t *testing.T) {
	t.Parallel()
	const endpoint = "/api.Service/Action"

	tests := []struct {
		name   string
		policy Policy
		rule   *AccessRule
		geo    GeoInfo
		want   bool
	}{
		{"deny list, unknown location", PolicyAllow, &AccessRule{DenyCountries: []string{"RU"}}, GeoInfo{}, true},
		{"deny continents, unknown location", PolicyAllow, &AccessRule{DenyContinents: []string{"AS"}}, GeoInfo{}, true},
		{"deny regions, unknown location", PolicyAllow, &AccessRule{DenyRegions: []string{"US-TX"}}, GeoInfo{}, true},
		{"allow list, unknown location", PolicyAllow, &AccessRule{AllowCountries: []string{"US"}}, GeoInfo{}, false},
		{"region without a country is unknown", PolicyAllow, &AccessRule{DenyCountries: []string{"RU"}}, GeoInfo{RegionCode: "TX"}, true},
		{"rule without lists keeps the registry policy", PolicyAllow, &AccessRule{}, GeoInfo{}, true},
		{"rule without lists keeps the deny policy", PolicyDeny, &AccessRule{}, GeoInfo{}, false},
		{"deny list, known location outside it", PolicyAllow, &AccessRule{DenyCountries: []string{"RU"}}, GeoInfo{CountryCode: "DE"}, true},
		{"deny list, continent only", PolicyAllow, &AccessRule{DenyCountries: []string{"RU"}}, GeoInfo{ContinentCode: "EU"}, true},
		{"deny list under PolicyDeny, known location outside it", PolicyDeny, &AccessRule{DenyCountries: []string{"RU"}}, GeoInfo{CountryCode: "DE"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := NewRegistry(tc.policy)
			reg.Register(endpoint, tc.rule)

			got, err := reg.Evaluate(t.Context(), &mockResolver{geo: tc.geo}, testIP, endpoint)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// Without a matching rule the resolver is not consulted and the registry
// policy decides, as before.
func TestEvaluate_NoRuleKeepsRegistryPolicy(t *testing.T) {
	t.Parallel()
	got, err := NewRegistry(PolicyAllow).Evaluate(t.Context(), &mockResolver{}, testIP, "/other.Service/Method")
	require.NoError(t, err)
	require.True(t, got)
}
