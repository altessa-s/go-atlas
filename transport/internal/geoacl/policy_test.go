// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"context"
	"errors"
	"net/netip"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

var testIP = netip.MustParseAddr("1.2.3.4")

type mockResolver struct {
	geo GeoInfo
	err error
}

func (m *mockResolver) Resolve(_ context.Context, _ netip.Addr) (GeoInfo, error) {
	return m.geo, m.err
}

// evaluateCase is one Evaluate expectation checked by runEvaluateCases.
type evaluateCase struct {
	name     string
	geo      GeoInfo
	endpoint string
	want     bool
}

// runEvaluateCases asserts, per subtest, that evaluating the endpoint against
// the registry for a resolver returning the case's geo yields the wanted verdict.
func runEvaluateCases(t *testing.T, reg *Registry, cases []evaluateCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, tt.endpoint)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_AllowCountriesMode(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/admin.AdminService/Delete", &AccessRule{
		AllowCountries: []string{"US", "CA"},
	})

	runEvaluateCases(t, reg, []evaluateCase{
		{"allowed country US", GeoInfo{CountryCode: "US"}, "/admin.AdminService/Delete", true},
		{"allowed country CA", GeoInfo{CountryCode: "CA"}, "/admin.AdminService/Delete", true},
		{"denied country RU", GeoInfo{CountryCode: "RU"}, "/admin.AdminService/Delete", false},
		{"no rule, policy deny", GeoInfo{CountryCode: "US"}, "/other.Service/Method", false},
	})
}

func TestEvaluate_DenyCountriesMode(t *testing.T) {
	reg := NewRegistry(PolicyAllow)
	reg.Register("/api.Service/Action", &AccessRule{
		DenyCountries: []string{"RU", "CN"},
	})

	runEvaluateCases(t, reg, []evaluateCase{
		{"denied country RU", GeoInfo{CountryCode: "RU"}, "/api.Service/Action", false},
		{"denied country CN", GeoInfo{CountryCode: "CN"}, "/api.Service/Action", false},
		{"allowed country US", GeoInfo{CountryCode: "US"}, "/api.Service/Action", true},
		{"no rule, policy allow", GeoInfo{CountryCode: "RU"}, "/other.Service/Method", true},
	})
}

func TestEvaluate_AllowContinentsMode(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/api.Service/Action", &AccessRule{
		AllowContinents: []string{"EU", "NA"},
	})

	tests := []struct {
		name string
		geo  GeoInfo
		want bool
	}{
		{"allowed continent EU", GeoInfo{ContinentCode: "EU"}, true},
		{"allowed continent NA", GeoInfo{ContinentCode: "NA"}, true},
		{"denied continent AS", GeoInfo{ContinentCode: "AS"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_DenyContinentsMode(t *testing.T) {
	reg := NewRegistry(PolicyAllow)
	reg.Register("/api.Service/Action", &AccessRule{
		DenyContinents: []string{"AS"},
	})

	tests := []struct {
		name string
		geo  GeoInfo
		want bool
	}{
		{"denied continent AS", GeoInfo{ContinentCode: "AS"}, false},
		{"allowed continent EU", GeoInfo{ContinentCode: "EU"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_DenyWinsOnOverlap(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/api.Service/Action", &AccessRule{
		AllowCountries: []string{"RU", "US"},
		DenyCountries:  []string{"RU"},
	})

	tests := []struct {
		name string
		geo  GeoInfo
		want bool
	}{
		{"in both lists, deny wins", GeoInfo{CountryCode: "RU"}, false},
		{"only in allowlist", GeoInfo{CountryCode: "US"}, true},
		{"in neither", GeoInfo{CountryCode: "DE"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_RegionPrecedence(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/api.Service/Action", &AccessRule{
		AllowCountries: []string{"US"},
		DenyRegions:    []string{"US-TX"},
	})

	tests := []struct {
		name string
		geo  GeoInfo
		want bool
	}{
		{"region deny overrides country allow", GeoInfo{CountryCode: "US", RegionCode: "TX"}, false},
		{"other region allowed via country", GeoInfo{CountryCode: "US", RegionCode: "CA"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_ContinentDenyOverridesCountryAllow(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/api.Service/Action", &AccessRule{
		AllowCountries: []string{"CN"},
		DenyContinents: []string{"AS"},
	})

	resolver := &mockResolver{geo: GeoInfo{ContinentCode: "AS", CountryCode: "CN"}}
	got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
	require.NoError(t, err)
	require.False(t, got, "expected deny: continent deny should override country allow")
}

func TestEvaluate_AllowRegionsMode(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/api.Service/Action", &AccessRule{
		AllowRegions: []string{"US-CA"},
	})

	tests := []struct {
		name string
		geo  GeoInfo
		want bool
	}{
		{"allowed region US-CA", GeoInfo{CountryCode: "US", RegionCode: "CA"}, true},
		{"denied region US-TX", GeoInfo{CountryCode: "US", RegionCode: "TX"}, false},
		{"denied region DE-BY", GeoInfo{CountryCode: "DE", RegionCode: "BY"}, false},
		{"no region info", GeoInfo{CountryCode: "US"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_DenyRegionsMode(t *testing.T) {
	reg := NewRegistry(PolicyAllow)
	reg.Register("/api.Service/Action", &AccessRule{
		DenyRegions: []string{"US-TX"},
	})

	tests := []struct {
		name string
		geo  GeoInfo
		want bool
	}{
		{"denied region US-TX", GeoInfo{CountryCode: "US", RegionCode: "TX"}, false},
		{"allowed region US-CA", GeoInfo{CountryCode: "US", RegionCode: "CA"}, true},
		{"allowed no region", GeoInfo{CountryCode: "US"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_EmptyGeoInfo_FallsBackToPolicy(t *testing.T) {
	t.Run("PolicyDeny", func(t *testing.T) {
		reg := NewRegistry(PolicyDeny)
		reg.Register("/api.Service/Action", &AccessRule{
			AllowCountries: []string{"US"},
		})
		resolver := &mockResolver{geo: GeoInfo{}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
		require.NoError(t, err)
		require.False(t, got, "expected deny when GeoInfo is empty and PolicyDeny")
	})

	t.Run("PolicyAllow", func(t *testing.T) {
		reg := NewRegistry(PolicyAllow)
		reg.Register("/api.Service/Action", &AccessRule{
			DenyCountries: []string{"RU"},
		})
		resolver := &mockResolver{geo: GeoInfo{}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
		require.NoError(t, err)
		require.True(t, got, "expected allow when GeoInfo is empty and PolicyAllow")
	})
}

func TestEvaluate_PatternMatch(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.RegisterPattern(regexp.MustCompile(`^/internal\..*`), &AccessRule{
		AllowCountries: []string{"US"},
	})

	tests := []struct {
		name     string
		geo      GeoInfo
		endpoint string
		want     bool
	}{
		{"pattern match, allowed", GeoInfo{CountryCode: "US"}, "/internal.Svc/Do", true},
		{"pattern match, denied", GeoInfo{CountryCode: "RU"}, "/internal.Svc/Do", false},
		{"no pattern match", GeoInfo{CountryCode: "US"}, "/public.Svc/Do", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, tt.endpoint)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_DefaultRule(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.SetDefault(&AccessRule{
		AllowContinents: []string{"EU", "NA"},
	})

	tests := []struct {
		name string
		geo  GeoInfo
		want bool
	}{
		{"default allows EU", GeoInfo{ContinentCode: "EU"}, true},
		{"default allows NA", GeoInfo{ContinentCode: "NA"}, true},
		{"default denies AS", GeoInfo{ContinentCode: "AS"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, "/any.Endpoint")
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluate_ResolverError(t *testing.T) {
	reg := NewRegistry(PolicyAllow)
	reg.Register("/api.Service/Action", &AccessRule{
		AllowCountries: []string{"US"},
	})

	resolverErr := errors.New("geo lookup failed")
	resolver := &mockResolver{err: resolverErr}

	_, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
	require.True(t, errors.Is(err, resolverErr))
}

func TestEvaluate_NoRule_PolicyApplies(t *testing.T) {
	t.Run("PolicyDeny", func(t *testing.T) {
		reg := NewRegistry(PolicyDeny)
		resolver := &mockResolver{geo: GeoInfo{CountryCode: "US"}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/some.Endpoint")
		require.NoError(t, err)
		require.False(t, got, "expected deny when no rule and PolicyDeny")
	})

	t.Run("PolicyAllow", func(t *testing.T) {
		reg := NewRegistry(PolicyAllow)
		resolver := &mockResolver{geo: GeoInfo{CountryCode: "US"}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/some.Endpoint")
		require.NoError(t, err)
		require.True(t, got, "expected allow when no rule and PolicyAllow")
	})
}

func TestEvaluate_EmptyLists_FallsBackToPolicy(t *testing.T) {
	t.Run("PolicyDeny", func(t *testing.T) {
		reg := NewRegistry(PolicyDeny)
		reg.Register("/api.Service/Action", &AccessRule{})
		resolver := &mockResolver{geo: GeoInfo{CountryCode: "US"}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
		require.NoError(t, err)
		require.False(t, got, "expected deny when rule has empty lists and PolicyDeny")
	})

	t.Run("PolicyAllow", func(t *testing.T) {
		reg := NewRegistry(PolicyAllow)
		reg.Register("/api.Service/Action", &AccessRule{})
		resolver := &mockResolver{geo: GeoInfo{CountryCode: "US"}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
		require.NoError(t, err)
		require.True(t, got, "expected allow when rule has empty lists and PolicyAllow")
	})
}

func TestLookup_ExactBeforePattern(t *testing.T) {
	reg := NewRegistry(PolicyDeny)

	exactRule := &AccessRule{AllowCountries: []string{"US"}}
	patRule := &AccessRule{AllowContinents: []string{"EU"}}

	reg.Register("/svc.Service/Method", exactRule)
	reg.RegisterPattern(regexp.MustCompile(`^/svc\..*`), patRule)

	got, ok := reg.Lookup("/svc.Service/Method")
	require.True(t, ok, "expected to find a rule")
	require.False(t, got != exactRule, "exact match should take priority over pattern match")
}

func TestRegisterEndpoints(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	rule := &AccessRule{AllowCountries: []string{"US"}}
	reg.RegisterEndpoints(rule, "/a.Svc/A", "/b.Svc/B")

	for _, ep := range []string{"/a.Svc/A", "/b.Svc/B"} {
		got, ok := reg.Lookup(ep)
		require.True(t, ok, "expected rule for endpoint %s", ep)
		require.Equal(t, rule, got)
	}
}

func TestGeoInfo_FullRegion(t *testing.T) {
	tests := []struct {
		name string
		geo  GeoInfo
		want string
	}{
		{"full", GeoInfo{CountryCode: "US", RegionCode: "CA"}, "US-CA"},
		{"no region", GeoInfo{CountryCode: "US"}, ""},
		{"no country", GeoInfo{RegionCode: "CA"}, ""},
		{"empty", GeoInfo{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.geo.FullRegion()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParsePolicy(t *testing.T) {
	tests := []struct {
		input string
		want  Policy
	}{
		{"allow", PolicyAllow},
		{"deny", PolicyDeny},
		{"unknown", PolicyDeny},
		{"", PolicyDeny},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParsePolicy(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}
