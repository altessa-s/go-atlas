// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/auth/oidc/rules"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

func policyTestProvider(t testing.TB, idp *coverageIdP, opts ...oidc.Option) *oidc.Provider {
	t.Helper()
	base := []oidc.Option{oidc.WithHTTPClient(idp.srv.Client()), oidc.WithDiscoveryAllowLoopbackHTTP(), oidc.WithDefaultValidationOptions(oidc.WithValidationAudience("svc"))}
	p, err := oidc.NewProvider(t.Context(), idp.discoveryURL(), append(base, opts...)...)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

func TestServiceConfig_AnyOfScopes(t *testing.T) {
	t.Parallel()

	cfg := &authconfig.ValidationRulesConfig{Scopes: &authconfig.ScopesValidationConfig{AnyOf: []string{"admin", "ops"}}}
	opts, err := ValidationOptions(cfg)
	require.NoError(t, err)

	idp := newCoverageIdP(t)
	p := policyTestProvider(t, idp, oidc.WithDefaultValidationOptions(append(opts, oidc.WithValidationAudience("svc"))...))

	tests := []struct {
		name  string
		scope any
		ok    bool
	}{
		{name: "space-delimited match", scope: "read ops", ok: true},
		{name: "list match", scope: []string{"admin"}, ok: true},
		{name: "no match", scope: "read write"},
		{name: "missing scope", scope: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := idp.sign(t, map[string]any{"scope": tc.scope})
			_, err := p.ValidateToken(t.Context(), raw)
			if tc.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, oidc.ErrCELValidation)
		})
	}
}

func TestServiceConfig_AnyOfScopesAndCustomCELRulesBothEnforced(t *testing.T) {
	t.Parallel()

	cfg := &authconfig.ValidationRulesConfig{
		Scopes:   &authconfig.ScopesValidationConfig{AnyOf: []string{"admin"}},
		CELRules: []authconfig.CELRuleDefinition{{Name: "users-only", Expression: "claims.role == 'user'"}},
	}
	opts, err := ValidationOptions(cfg)
	require.NoError(t, err)

	idp := newCoverageIdP(t)
	p := policyTestProvider(t, idp, oidc.WithTokenCache(&mapCacher{items: map[string][]byte{}}),
		oidc.WithDefaultValidationOptions(append(opts, oidc.WithValidationAudience("svc"))...))

	noScope := idp.sign(t, map[string]any{"role": "user", "scope": "read"})
	wrongRole := idp.sign(t, map[string]any{"role": "guest", "scope": "admin"})
	both := idp.sign(t, map[string]any{"role": "user", "scope": "admin"})

	for range 2 { // second round exercises cache hits
		_, err = p.ValidateToken(t.Context(), noScope)
		require.ErrorIs(t, err, oidc.ErrCELValidation, "any_of must survive custom CEL rules")
		_, err = p.ValidateToken(t.Context(), wrongRole)
		require.ErrorIs(t, err, oidc.ErrCELValidation)
		_, err = p.ValidateToken(t.Context(), both)
		require.NoError(t, err)
	}
}

func TestExactPolicyFreshAndCached(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	const adjacent = "1750000000.000000002"
	for _, tc := range []struct {
		name, iat, condition, expected, cel string
		want                                error
	}{
		{name: "date literal", iat: adjacent, condition: `{"claim_equals":{"iat":1750000000.000000002}}`},
		{name: "adjacent literal", iat: adjacent, condition: `{"claim_equals":{"iat":1750000000.000000001}}`, want: oidc.ErrTokenInvalid},
		{name: "strict quoted", iat: "1750000000", condition: `{"claim_equals":{"iat":"1750000000"}}`, want: oidc.ErrTokenInvalid},
		{name: "expected literal", iat: adjacent, expected: `1750000000.000000002`},
		{name: "expected adjacent", iat: adjacent, expected: `1750000000.000000001`, want: oidc.ErrTokenInvalid},
		{name: "coerced", iat: "1750000000", expected: `"1750000000"`},
		{name: "numeric string syntax", iat: "1750000000", expected: `"+01750000000.0"`},
		{name: "coerced adjacent", iat: adjacent, expected: `"1750000000.000000001"`, want: oidc.ErrTokenInvalid},
		{name: "billion", iat: "1", condition: `{"claim_equals":{"billion":1000000000}}`},
		{name: "integral float", iat: "1", condition: `{"claim_equals":{"one":1.0}}`},
		{name: "rounded custom number", iat: "1", condition: `{"claim_equals":{"large":9007199254740993}}`},
		{name: "CEL integer", iat: "1750000000", cel: "type(claims.iat) == double && claims.iat == 1750000000.0"},
		{name: "CEL exponent", iat: "1.75e9", cel: "type(claims.iat) == double && claims.iat == 1750000000.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// A matching rule selects a policy for the actual subject. The
			// distinguishable default fails, making preset selection observable.
			condition := tc.condition
			preset := `{"audiences":["svc"],"subject":"user-1"}`
			if tc.cel != "" {
				condition = fmt.Sprintf(`{"cel_expression":%q}`, tc.cel)
				preset = fmt.Sprintf(`{"audiences":["svc"],"cel_rules":[{"name":"double","expression":%q}]}`, tc.cel)
			}
			if tc.expected != "" {
				condition = `{"claim_exists":["sub"]}`
				preset = fmt.Sprintf(`{"audiences":["svc"],"expected_claims":{"iat":%s}}`, tc.expected)
			}
			data := fmt.Sprintf(`{"version":"1.0","default_validation":{"audiences":["svc"],"subject":"default"},
				"presets":[{"name":"selected","validation":%s}],"preset_rules":[{"preset":"selected","conditions":%s}]}`, preset, condition)
			cfg, err := authconfig.ParseServiceConfig([]byte(data))
			require.NoError(t, err)
			opts, err := ServiceConfigOptions(cfg)
			require.NoError(t, err)
			cache := &mapCacher{items: map[string][]byte{}}
			p := policyTestProvider(t, idp, append(opts, oidc.WithTokenCache(cache))...)
			raw := idp.sign(t, map[string]any{"iat": json.Number(tc.iat), "one": 1, "billion": 1e9, "large": int64(9007199254740993)})
			for range 2 {
				got, err := p.ValidateToken(t.Context(), raw)
				require.ErrorIs(t, err, tc.want)
				if err == nil {
					require.Equal(t, json.Number(tc.iat), got["iat"])
				}
			}
			require.Equal(t, 1, cache.saveCount())
		})
	}
}

func TestValidationRulesConfig_VerifyNotBefore_NilOmitsRequirement(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	opts, err := ValidationOptions(&authconfig.ValidationRulesConfig{Audiences: []string{"svc"}})
	require.NoError(t, err)
	p := policyTestProvider(t, idp, oidc.WithDefaultValidationOptions(opts...))
	_, err = p.ValidateToken(t.Context(), idp.sign(t, nil))
	require.NoError(t, err)
}

func TestValidationRulesConfig_VerifyNotBefore_TrueRequiresNbf(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	opts, err := ValidationOptions(&authconfig.ValidationRulesConfig{Audiences: []string{"svc"}, VerifyNotBefore: new(true)})
	require.NoError(t, err)
	p := policyTestProvider(t, idp, oidc.WithDefaultValidationOptions(opts...))
	_, err = p.ValidateToken(t.Context(), idp.sign(t, nil))
	require.ErrorIs(t, err, oidc.ErrTokenInvalid)
}

func TestValidationRulesConfig_ExpectedClaimsKeepTypes(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	opts, err := ValidationOptions(&authconfig.ValidationRulesConfig{Audiences: []string{"svc"}, ExpectedClaims: map[string]any{"email_verified": true, "tier": float64(2)}})
	require.NoError(t, err)
	p := policyTestProvider(t, idp, oidc.WithDefaultValidationOptions(opts...))
	_, err = p.ValidateToken(t.Context(), idp.sign(t, map[string]any{"email_verified": true, "tier": 2}))
	require.NoError(t, err)
	_, err = p.ValidateToken(t.Context(), idp.sign(t, map[string]any{"email_verified": "true", "tier": 2}))
	require.ErrorIs(t, err, oidc.ErrTokenInvalid)
	_, err = p.ValidateToken(t.Context(), idp.sign(t, map[string]any{"email_verified": true, "tier": "2"}))
	require.ErrorIs(t, err, oidc.ErrTokenInvalid)
}

func TestLoadServiceConfigOptionsPrecedence(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	path := filepath.Join(t.TempDir(), "service.json")
	const doc = `{"version":"1.0","default_validation":{"audiences":["svc"],"token_lifetime":{"min":"48h"}},
 "presets":[{"name":"named","validation":{"audiences":["svc"],"subject":"user-1"}}],
 "preset_rules":[{"preset":"named","conditions":{"has_scope":"read"}}]}`
	require.NoError(t, os.WriteFile(path, []byte(doc), 0600))
	opts, err := LoadServiceConfigOptions(" "+path+" ",
		oidc.WithDefaultValidationOptions(oidc.WithValidationAudience("caller")),
		oidc.WithPresets(oidc.NewValidationPreset("named", oidc.WithValidationAudience("caller")),
			oidc.NewValidationPreset("caller-only", oidc.WithValidationAudience("svc"))),
		oidc.WithPresetRules(oidc.PresetRule{PresetName: "named", Matcher: oidc.ClaimExists("sub")}),
	)
	require.NoError(t, err)
	p := policyTestProvider(t, idp, opts...)
	// The file minimum rejects the one-hour token under the default policy.
	token := idp.sign(t, map[string]any{"sub": "default"})
	_, err = p.ValidateToken(t.Context(), token)
	require.ErrorIs(t, err, oidc.ErrTokenInvalid)
	// The file rule selects the file's preset of the same name.
	_, err = p.ValidateToken(t.Context(), idp.sign(t, map[string]any{"sub": "default", "scope": "read"}))
	require.ErrorIs(t, err, oidc.ErrTokenInvalid)
	_, err = p.ValidateTokenWithPreset(t.Context(), idp.sign(t, nil), "named")
	require.NoError(t, err)
	_, err = p.ValidateTokenWithPreset(t.Context(), token, "caller-only")
	require.NoError(t, err)
}

func TestLoadServiceConfigOptionsErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, doc string
		want      error
	}{
		{"absent", "", os.ErrNotExist},
		{"version", `{"version":"2.0"}`, authconfig.ErrInvalidVersion},
		{"minimum", `{"version":"1.0","default_validation":{"token_lifetime":{"min":"later"}}}`, authconfig.ErrInvalidDuration},
		{"preset", `{"version":"1.0","preset_rules":[{"preset":"missing"}]}`, authconfig.ErrPresetNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "service.json")
			if tc.doc != "" {
				require.NoError(t, os.WriteFile(path, []byte(tc.doc), 0600))
			}
			_, err := LoadServiceConfigOptions(path)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestServiceConfigMinimumLifetimeFreshAndCached(t *testing.T) {
	t.Parallel()
	const doc = `{"version":"1.0",
		"default_validation":{"audiences":["svc"],"token_lifetime":{"min":"90m","max":"2h"}},
		"presets":[{"name":"selected","validation":{"audiences":["svc"],"token_lifetime":{"min":"30m","max":"2h"}}}],
		"preset_rules":[{"preset":"selected","conditions":{"has_scope":"read"}}]}`
	cfg, err := authconfig.ParseServiceConfig([]byte(doc))
	require.NoError(t, err)
	opts, err := ServiceConfigOptions(cfg)
	require.NoError(t, err)

	idp := newCoverageIdP(t)
	now := time.Now()
	for _, tc := range []struct {
		name  string
		scope string
		ttl   time.Duration
		want  error
	}{
		{"default rejects one hour", "", time.Hour, oidc.ErrTokenInvalid},
		{"preset accepts one hour", "read", time.Hour, nil},
		{"preset rejects fifteen minutes", "read", 15 * time.Minute, oidc.ErrTokenInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cache := &mapCacher{items: map[string][]byte{}}
			p := policyTestProvider(t, idp, slices.Concat(opts, []oidc.Option{oidc.WithTokenCache(cache)})...)
			token := idp.sign(t, map[string]any{"scope": tc.scope, "iat": now.Unix(), "exp": now.Add(tc.ttl).Unix()})
			_, freshErr := p.ValidateToken(t.Context(), token)
			_, cachedErr := p.ValidateToken(t.Context(), token)
			require.Equal(t, 1, cache.saveCount())
			t.Run("fresh", func(t *testing.T) {
				t.Parallel()
				require.ErrorIs(t, freshErr, tc.want)
			})
			t.Run("cached", func(t *testing.T) {
				t.Parallel()
				require.ErrorIs(t, cachedErr, tc.want)
			})
		})
	}
}

func TestValidationOptionsRejectsInvalidMinimumLifetime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, min, max string
		want           error
	}{
		{"malformed", "later", "", authconfig.ErrInvalidDuration},
		{"negative", "-1s", "", rules.ErrInvalidConfig},
		{"above maximum", "2h", "1h", rules.ErrInvalidConfig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ValidationOptions(&authconfig.ValidationRulesConfig{
				TokenLifetime: &authconfig.TokenLifetimeValidationConfig{Min: tc.min, Max: tc.max},
			})
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestValidationOptionsZeroMinimumDisablesDefault(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	p := policyTestProvider(t, idp, oidc.WithDefaultValidationOptions(
		oidc.WithValidationAudience("svc"), oidc.WithValidationMinTokenLifetime(2*time.Hour)))
	opts, err := ValidationOptions(&authconfig.ValidationRulesConfig{
		TokenLifetime: &authconfig.TokenLifetimeValidationConfig{Min: "0s"},
	})
	require.NoError(t, err)
	token := idp.sign(t, nil)
	_, err = p.ValidateToken(t.Context(), token)
	require.ErrorIs(t, err, oidc.ErrTokenInvalid)
	_, err = p.ValidateTokenWithOptions(t.Context(), token, opts...)
	require.NoError(t, err)
}
