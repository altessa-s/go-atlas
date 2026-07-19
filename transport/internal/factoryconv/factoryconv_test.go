// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factoryconv_test

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/internal/factoryconv"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/geoacl"
)

func TestCompilePatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{
			name:     "all valid",
			patterns: []string{`^/internal/.*$`, `/health`},
			want:     []string{`^/internal/.*$`, `/health`},
		},
		{
			name:     "invalid patterns are skipped",
			patterns: []string{`^/api/.*$`, `([`, `*invalid`},
			want:     []string{`^/api/.*$`},
		},
		{
			name:     "empty input",
			patterns: nil,
			want:     []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := factoryconv.CompilePatterns(tc.patterns)
			require.Len(t, got, len(tc.want))
			for i, re := range got {
				require.Equal(t, tc.want[i], re.String())
			}
		})
	}
}

func TestParsePrefixes(t *testing.T) {
	t.Parallel()

	t.Run("valid CIDR and bare IP", func(t *testing.T) {
		t.Parallel()

		prefixes, err := factoryconv.ParsePrefixes([]string{"10.0.0.0/8", "192.168.1.1"})
		require.NoError(t, err)
		require.Equal(t, []netip.Prefix{
			netip.MustParsePrefix("10.0.0.0/8"),
			netip.MustParsePrefix("192.168.1.1/32"),
		}, prefixes)
	})

	t.Run("invalid input", func(t *testing.T) {
		t.Parallel()

		_, err := factoryconv.ParsePrefixes([]string{"not-an-ip"})
		require.Error(t, err)
	})
}

func TestConvertFallbackBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   config.FallbackBehavior
		want fallback.Behavior
	}{
		{"allow", config.FallbackBehaviorAllow, fallback.Allow},
		{"deny", config.FallbackBehaviorDeny, fallback.Deny},
		{"error", config.FallbackBehaviorError, fallback.Error},
		{"unknown fails closed to deny", config.FallbackBehavior("bogus"), fallback.Deny},
		{"empty fails closed to deny", config.FallbackBehavior(""), fallback.Deny},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, factoryconv.ConvertFallbackBehavior(tc.in))
		})
	}
}

func TestConvertIpAclRule(t *testing.T) {
	t.Parallel()

	t.Run("valid rule", func(t *testing.T) {
		t.Parallel()

		rule, err := factoryconv.ConvertIpAclRule(config.IpAclRuleConfig{
			Allowlist: []string{"10.0.0.0/8"},
			Denylist:  []string{"192.168.1.1"},
		})
		require.NoError(t, err)
		require.Equal(t, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, rule.Allowlist)
		require.Equal(t, []netip.Prefix{netip.MustParsePrefix("192.168.1.1/32")}, rule.Denylist)
	})

	t.Run("empty rule", func(t *testing.T) {
		t.Parallel()

		rule, err := factoryconv.ConvertIpAclRule(config.IpAclRuleConfig{})
		require.NoError(t, err)
		require.Empty(t, rule.Allowlist)
		require.Empty(t, rule.Denylist)
	})

	t.Run("invalid allowlist", func(t *testing.T) {
		t.Parallel()

		_, err := factoryconv.ConvertIpAclRule(config.IpAclRuleConfig{Allowlist: []string{"bad"}})
		require.Error(t, err)
	})

	t.Run("invalid denylist", func(t *testing.T) {
		t.Parallel()

		_, err := factoryconv.ConvertIpAclRule(config.IpAclRuleConfig{Denylist: []string{"bad"}})
		require.Error(t, err)
	})
}

func TestBuildIpAclRegistry(t *testing.T) {
	t.Parallel()

	t.Run("endpoints, patterns and default rule", func(t *testing.T) {
		t.Parallel()

		registry, err := factoryconv.BuildIpAclRegistry("deny",
			[]config.IpAclRuleConfig{{
				Endpoints: []string{"/svc.Api/Get"},
				Patterns:  []string{`^/svc\.Api/List.*$`},
				Allowlist: []string{"10.0.0.0/8"},
			}},
			&config.IpAclRuleConfig{Denylist: []string{"192.168.0.0/16"}},
		)
		require.NoError(t, err)

		rule, ok := registry.Lookup("/svc.Api/Get")
		require.True(t, ok)
		require.Equal(t, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, rule.Allowlist)

		rule, ok = registry.Lookup("/svc.Api/ListItems")
		require.True(t, ok)
		require.Equal(t, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, rule.Allowlist)

		rule, ok = registry.Lookup("/svc.Api/Delete")
		require.True(t, ok)
		require.Equal(t, []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16")}, rule.Denylist)
	})

	t.Run("no default rule", func(t *testing.T) {
		t.Parallel()

		registry, err := factoryconv.BuildIpAclRegistry("allow", nil, nil)
		require.NoError(t, err)

		_, ok := registry.Lookup("/svc.Api/Get")
		require.False(t, ok)
	})

	t.Run("invalid endpoint pattern", func(t *testing.T) {
		t.Parallel()

		_, err := factoryconv.BuildIpAclRegistry("deny",
			[]config.IpAclRuleConfig{{Patterns: []string{`([`}}}, nil)
		require.Error(t, err)
	})

	t.Run("invalid rule prefix", func(t *testing.T) {
		t.Parallel()

		_, err := factoryconv.BuildIpAclRegistry("deny",
			[]config.IpAclRuleConfig{{Allowlist: []string{"bad"}}}, nil)
		require.Error(t, err)
	})

	t.Run("invalid default rule prefix", func(t *testing.T) {
		t.Parallel()

		_, err := factoryconv.BuildIpAclRegistry("deny", nil,
			&config.IpAclRuleConfig{Denylist: []string{"bad"}})
		require.Error(t, err)
	})
}

func TestConvertGeoAclRule(t *testing.T) {
	t.Parallel()

	rule := factoryconv.ConvertGeoAclRule(config.GeoAclRuleConfig{
		AllowContinents: []string{"EU"},
		DenyContinents:  []string{"AN"},
		AllowCountries:  []string{"DE", "FR"},
		DenyCountries:   []string{"XX"},
		AllowRegions:    []string{"US-CA"},
		DenyRegions:     []string{"US-TX"},
	})

	require.Equal(t, &geoacl.AccessRule{
		AllowContinents: []string{"EU"},
		DenyContinents:  []string{"AN"},
		AllowCountries:  []string{"DE", "FR"},
		DenyCountries:   []string{"XX"},
		AllowRegions:    []string{"US-CA"},
		DenyRegions:     []string{"US-TX"},
	}, rule)
}

func TestBuildGeoAclRegistry(t *testing.T) {
	t.Parallel()

	t.Run("endpoints, patterns and default rule", func(t *testing.T) {
		t.Parallel()

		registry, err := factoryconv.BuildGeoAclRegistry("deny",
			[]config.GeoAclRuleConfig{{
				Endpoints:      []string{"/svc.Api/Get"},
				Patterns:       []string{`^/svc\.Api/List.*$`},
				AllowCountries: []string{"DE"},
			}},
			&config.GeoAclRuleConfig{DenyCountries: []string{"XX"}},
		)
		require.NoError(t, err)

		rule, ok := registry.Lookup("/svc.Api/Get")
		require.True(t, ok)
		require.Equal(t, []string{"DE"}, rule.AllowCountries)

		rule, ok = registry.Lookup("/svc.Api/ListItems")
		require.True(t, ok)
		require.Equal(t, []string{"DE"}, rule.AllowCountries)

		rule, ok = registry.Lookup("/svc.Api/Delete")
		require.True(t, ok)
		require.Equal(t, []string{"XX"}, rule.DenyCountries)
	})

	t.Run("no default rule", func(t *testing.T) {
		t.Parallel()

		registry, err := factoryconv.BuildGeoAclRegistry("allow", nil, nil)
		require.NoError(t, err)

		_, ok := registry.Lookup("/svc.Api/Get")
		require.False(t, ok)
	})

	t.Run("invalid endpoint pattern", func(t *testing.T) {
		t.Parallel()

		_, err := factoryconv.BuildGeoAclRegistry("deny",
			[]config.GeoAclRuleConfig{{Patterns: []string{`([`}}}, nil)
		require.Error(t, err)
	})
}
