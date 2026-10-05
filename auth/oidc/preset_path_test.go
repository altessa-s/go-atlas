// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

const (
	cacheHitsMetric        = "test_auth_oidc_cache_hits_total"
	cacheMissesMetric      = "test_auth_oidc_cache_misses_total"
	tokenValidationsMetric = "test_auth_oidc_token_validations_total"
	validationErrorsMetric = "test_auth_oidc_validation_errors_total"
	validationTimeMetric   = "test_auth_oidc_validation_duration_seconds"
)

// When preset rules are configured but none selects a registered preset, the
// default validation reuses the signature verification done for selection:
// a fresh token costs one cache miss, not a miss plus a hit.
func TestValidate_UnmatchedPresetRuleVerifiesOnce(t *testing.T) {
	t.Parallel()

	never := func(map[string]any) bool { return false }
	always := func(map[string]any) bool { return true }
	for name, rule := range map[string]PresetRule{
		"no rule matches":    {PresetName: "strict", Matcher: never},
		"unregistered match": {PresetName: "missing", Matcher: always},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			collector := testhelpers.NewTestCollector()
			strict := NewValidationPreset("strict", WithValidationAudience(testAudience))
			p := idp.newProvider(t, WithCollector(collector), WithTokenCache(newMemCacher()),
				WithPresets(strict), WithPresetRules(rule))

			raw := idp.sign(t, idp.claims(nil))
			_, err := p.ValidateToken(t.Context(), raw)
			require.NoError(t, err)
			require.Equal(t, 1.0, testhelpers.GetCounterValue(t, collector, cacheMissesMetric))
			require.Zero(t, testhelpers.GetCounterValue(t, collector, cacheHitsMetric), "one verification per validation")

			_, err = p.ValidateToken(t.Context(), raw)
			require.NoError(t, err)
			require.Equal(t, 1.0, testhelpers.GetCounterValue(t, collector, cacheHitsMetric))
			require.Equal(t, 1.0, testhelpers.GetCounterValue(t, collector, cacheMissesMetric))

			// The default policy is still enforced on the reused verification.
			_, err = p.ValidateToken(t.Context(), idp.sign(t, idp.claims(map[string]any{"aud": "other"})))
			require.ErrorIs(t, err, ErrTokenInvalid)

			narrow := idp.newProvider(t, WithPresets(strict), WithPresetRules(rule),
				WithDefaultValidationOptions(WithValidationAudience(testAudience), WithValidationValidMethods("ES256")))
			_, err = narrow.ValidateToken(t.Context(), raw)
			require.ErrorIs(t, err, ErrTokenInvalid, "the default signing-algorithm allow-list applies")
		})
	}
}

// Preset matchers get a copy of the verified claims: whatever they change
// never reaches the policy, the revocation lookup or the returned claims.
func TestValidate_PresetMatcherCannotAlterValidatedClaims(t *testing.T) {
	t.Parallel()

	tamper := func(selected bool) PresetMatcherFunc {
		return func(c map[string]any) bool {
			c["aud"] = testAudience
			c["injected"] = true
			if groups, ok := c["groups"].([]any); ok && len(groups) > 0 {
				groups[0] = "admin"
			}
			if meta, ok := c["meta"].(map[string]any); ok {
				meta["role"] = "admin"
			}
			return selected
		}
	}
	tests := map[string]PresetRule{
		"no rule matches":    {PresetName: "lenient", Matcher: tamper(false)},
		"unregistered match": {PresetName: "missing", Matcher: tamper(true)},
		"registered match":   {PresetName: "lenient", Matcher: tamper(true)},
	}
	for name, rule := range tests {
		for _, cached := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cache=%t", name, cached), func(t *testing.T) {
				t.Parallel()

				idp := newTestIdP(t)
				opts := []Option{
					WithPresets(NewValidationPreset("lenient", WithValidationAudience(testAudience))),
					WithPresetRules(rule),
				}
				if cached {
					opts = append(opts, WithTokenCache(newMemCacher()))
				}
				p := idp.newProvider(t, opts...)

				extra := map[string]any{"groups": []any{"user"}, "meta": map[string]any{"role": "user"}}
				for round := range 2 { // second round: cache hit when cached
					wrongAud := idp.claims(map[string]any{"aud": "other", "groups": []any{"user"}})
					_, err := p.ValidateToken(t.Context(), idp.sign(t, wrongAud))
					require.ErrorIs(t, err, ErrTokenInvalid, "round %d", round)

					raw := idp.sign(t, idp.claims(extra))
					claims, err := p.ValidateToken(t.Context(), raw)
					require.NoError(t, err)
					require.NotContains(t, claims, "injected")
					require.Equal(t, []any{"user"}, claims["groups"])
					require.Equal(t, map[string]any{"role": "user"}, claims["meta"])
				}
			})
		}
	}
}

// Every public validation entry point counts one attempt, one duration
// sample and, on failure, one error.
func TestValidationMetrics_AllEntryPoints(t *testing.T) {
	t.Parallel()

	matchAll := PresetRule{PresetName: "svc", Matcher: func(map[string]any) bool { return true }}
	matchNone := PresetRule{PresetName: "svc", Matcher: func(map[string]any) bool { return false }}
	tests := []struct {
		name     string
		rules    []PresetRule
		badToken bool
		call     func(ctx context.Context, p *Provider, token string) error
		wantErr  bool
	}{
		{name: "ValidateToken", call: func(ctx context.Context, p *Provider, tok string) error {
			_, err := p.ValidateToken(ctx, tok)
			return err
		}},
		{name: "ValidateToken invalid", badToken: true, wantErr: true, call: func(ctx context.Context, p *Provider, tok string) error {
			_, err := p.ValidateToken(ctx, tok)
			return err
		}},
		{name: "ValidateTokenWithOptions", call: func(ctx context.Context, p *Provider, tok string) error {
			_, err := p.ValidateTokenWithOptions(ctx, tok, WithValidationAudience(testAudience))
			return err
		}},
		{name: "auto-selected preset", rules: []PresetRule{matchAll}, call: func(ctx context.Context, p *Provider, tok string) error {
			_, err := p.ValidateToken(ctx, tok)
			return err
		}},
		{name: "auto-selected preset invalid", rules: []PresetRule{matchAll}, badToken: true, wantErr: true,
			call: func(ctx context.Context, p *Provider, tok string) error {
				_, err := p.ValidateToken(ctx, tok)
				return err
			}},
		{name: "unmatched preset rule", rules: []PresetRule{matchNone}, call: func(ctx context.Context, p *Provider, tok string) error {
			_, err := p.ValidateToken(ctx, tok)
			return err
		}},
		{name: "ValidateTokenWithPreset", call: func(ctx context.Context, p *Provider, tok string) error {
			_, err := p.ValidateTokenWithPreset(ctx, tok, "svc")
			return err
		}},
		{name: "ValidateTokenWithPreset invalid", badToken: true, wantErr: true, call: func(ctx context.Context, p *Provider, tok string) error {
			_, err := p.ValidateTokenWithPreset(ctx, tok, "svc")
			return err
		}},
		{name: "ValidateTokenWithPreset with options", call: func(ctx context.Context, p *Provider, tok string) error {
			_, err := p.ValidateTokenWithPreset(ctx, tok, "svc", WithValidationRequiredClaims("sub"))
			return err
		}},
		{name: "ValidateTokenWithPreset unknown preset", wantErr: true, call: func(ctx context.Context, p *Provider, tok string) error {
			_, err := p.ValidateTokenWithPreset(ctx, tok, "missing")
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			collector := testhelpers.NewTestCollector()
			p := idp.newProvider(t, WithCollector(collector),
				WithPresets(NewValidationPreset("svc", WithValidationAudience(testAudience))), WithPresetRules(tc.rules...))

			tok := idp.sign(t, idp.claims(nil))
			if tc.badToken {
				tok = "not-a-jwt"
			}
			err := tc.call(t.Context(), p, tok)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			issuer := []string{"issuer", idp.srv.URL}
			require.Equal(t, 1.0, testhelpers.GetCounterValue(t, collector, tokenValidationsMetric, issuer...))
			wantErrors := 0.0
			if tc.wantErr {
				wantErrors = 1
			}
			require.Equal(t, wantErrors, testhelpers.GetCounterValue(t, collector, validationErrorsMetric, issuer...))
			require.EqualValues(t, 1, testhelpers.GetHistogramCount(t, collector, validationTimeMetric))
		})
	}
}
