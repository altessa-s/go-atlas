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
)

var testIP = netip.MustParseAddr("1.2.3.4")

type mockResolver struct {
	geo GeoInfo
	err error
}

func (m *mockResolver) Resolve(_ context.Context, _ netip.Addr) (GeoInfo, error) {
	return m.geo, m.err
}

func TestEvaluate_AllowCountriesMode(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/admin.AdminService/Delete", &AccessRule{
		AllowCountries: []string{"US", "CA"},
	})

	tests := []struct {
		name     string
		geo      GeoInfo
		endpoint string
		want     bool
	}{
		{"allowed country US", GeoInfo{CountryCode: "US"}, "/admin.AdminService/Delete", true},
		{"allowed country CA", GeoInfo{CountryCode: "CA"}, "/admin.AdminService/Delete", true},
		{"denied country RU", GeoInfo{CountryCode: "RU"}, "/admin.AdminService/Delete", false},
		{"no rule, policy deny", GeoInfo{CountryCode: "US"}, "/other.Service/Method", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, tt.endpoint)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluate_DenyCountriesMode(t *testing.T) {
	reg := NewRegistry(PolicyAllow)
	reg.Register("/api.Service/Action", &AccessRule{
		DenyCountries: []string{"RU", "CN"},
	})

	tests := []struct {
		name     string
		geo      GeoInfo
		endpoint string
		want     bool
	}{
		{"denied country RU", GeoInfo{CountryCode: "RU"}, "/api.Service/Action", false},
		{"denied country CN", GeoInfo{CountryCode: "CN"}, "/api.Service/Action", false},
		{"allowed country US", GeoInfo{CountryCode: "US"}, "/api.Service/Action", true},
		{"no rule, policy allow", GeoInfo{CountryCode: "RU"}, "/other.Service/Method", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockResolver{geo: tt.geo}
			got, err := reg.Evaluate(t.Context(), resolver, testIP, tt.endpoint)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected deny: continent deny should override country allow")
	}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Error("expected deny when GeoInfo is empty and PolicyDeny")
		}
	})

	t.Run("PolicyAllow", func(t *testing.T) {
		reg := NewRegistry(PolicyAllow)
		reg.Register("/api.Service/Action", &AccessRule{
			DenyCountries: []string{"RU"},
		})
		resolver := &mockResolver{geo: GeoInfo{}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Error("expected allow when GeoInfo is empty and PolicyAllow")
		}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
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
	if !errors.Is(err, resolverErr) {
		t.Fatalf("expected resolver error, got: %v", err)
	}
}

func TestEvaluate_NoRule_PolicyApplies(t *testing.T) {
	t.Run("PolicyDeny", func(t *testing.T) {
		reg := NewRegistry(PolicyDeny)
		resolver := &mockResolver{geo: GeoInfo{CountryCode: "US"}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/some.Endpoint")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Error("expected deny when no rule and PolicyDeny")
		}
	})

	t.Run("PolicyAllow", func(t *testing.T) {
		reg := NewRegistry(PolicyAllow)
		resolver := &mockResolver{geo: GeoInfo{CountryCode: "US"}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/some.Endpoint")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Error("expected allow when no rule and PolicyAllow")
		}
	})
}

func TestEvaluate_EmptyLists_FallsBackToPolicy(t *testing.T) {
	t.Run("PolicyDeny", func(t *testing.T) {
		reg := NewRegistry(PolicyDeny)
		reg.Register("/api.Service/Action", &AccessRule{})
		resolver := &mockResolver{geo: GeoInfo{CountryCode: "US"}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Error("expected deny when rule has empty lists and PolicyDeny")
		}
	})

	t.Run("PolicyAllow", func(t *testing.T) {
		reg := NewRegistry(PolicyAllow)
		reg.Register("/api.Service/Action", &AccessRule{})
		resolver := &mockResolver{geo: GeoInfo{CountryCode: "US"}}
		got, err := reg.Evaluate(t.Context(), resolver, testIP, "/api.Service/Action")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Error("expected allow when rule has empty lists and PolicyAllow")
		}
	})
}

func TestLookup_ExactBeforePattern(t *testing.T) {
	reg := NewRegistry(PolicyDeny)

	exactRule := &AccessRule{AllowCountries: []string{"US"}}
	patRule := &AccessRule{AllowContinents: []string{"EU"}}

	reg.Register("/svc.Service/Method", exactRule)
	reg.RegisterPattern(regexp.MustCompile(`^/svc\..*`), patRule)

	got, ok := reg.Lookup("/svc.Service/Method")
	if !ok {
		t.Fatal("expected to find a rule")
	}
	if got != exactRule {
		t.Error("exact match should take priority over pattern match")
	}
}

func TestRegisterEndpoints(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	rule := &AccessRule{AllowCountries: []string{"US"}}
	reg.RegisterEndpoints(rule, "/a.Svc/A", "/b.Svc/B")

	for _, ep := range []string{"/a.Svc/A", "/b.Svc/B"} {
		got, ok := reg.Lookup(ep)
		if !ok || got != rule {
			t.Errorf("expected rule for endpoint %s", ep)
		}
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
			if got := tt.geo.FullRegion(); got != tt.want {
				t.Errorf("FullRegion() = %q, want %q", got, tt.want)
			}
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
			if got := ParsePolicy(tt.input); got != tt.want {
				t.Errorf("ParsePolicy(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
