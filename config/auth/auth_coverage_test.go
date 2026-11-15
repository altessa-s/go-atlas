// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func fieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func validOIDC() authconfig.OIDC {
	c := authconfig.DefaultOIDC()
	c.DiscoveryUrl = "https://idp.example.com/.well-known/openid-configuration"
	return c
}

func validRevocation() *authconfig.OIDCRevocation {
	r := authconfig.DefaultOIDCRevocation()
	r.Enabled = true
	r.Filter = &probfilterconfig.Filter{
		Type: probfilterconfig.TypeBloom, Bloom: &probfilterconfig.BloomConfig{ExpectedItems: 100},
	}
	r.Source = &authconfig.OIDCRevocationSource{URL: "https://idp.example.com/revoked"}
	return &r
}

func TestMTLS_Validate(t *testing.T) {
	t.Parallel()

	m := authconfig.DefaultMTLS()
	require.True(t, m.CheckExpiry)
	require.False(t, m.AllowAnyTrustDomain)
	require.Equal(t, authconfig.DefaultMTLSExpiryLeeway, m.ExpiryLeeway)
	m.IssuerKeyIDs = []string{"a1b2c3"}
	m.RequiredEKUs = []string{"clientAuth", "any"}
	require.NoError(t, m.Validate())

	m.IssuerKeyIDs = []string{"zz-not-hex"}
	m.RequiredEKUs = []string{"codeSigning"}
	verrs := fieldErrors(t, m.Validate())
	require.Contains(t, verrs, "IssuerKeyIDs")
	require.Contains(t, verrs, "RequiredEKUs")
}

func TestOIDC_Defaults(t *testing.T) {
	t.Parallel()

	c := validOIDC()
	require.NoError(t, c.Validate())
	require.False(t, c.IsCacheConfigured())
	require.False(t, c.IsClientCredentialsConfigured())
	require.False(t, c.IsIntrospectionEnabled())
	require.False(t, c.IsValidationConfigured())
	require.False(t, c.IsJWKSConfigured())
	require.False(t, c.IsPresetsConfigured())
	require.False(t, c.IsRevocationConfigured())

	cache := authconfig.DefaultOIDCCache()
	jwks := authconfig.DefaultOIDCJwks()
	c.Cache = &cache
	c.JWKS = &jwks
	c.Validation = &authconfig.OIDCValidation{Issuer: new("https://idp.example.com")}
	c.ClientCredentials = &authconfig.OIDCClientCredentials{ClientId: "id", ClientSecret: "secret"}
	c.Introspection = &authconfig.OIDCIntrospection{Enabled: true}
	c.Presets = &authconfig.OIDCPresets{}
	c.Revocation = validRevocation()
	require.NoError(t, c.Validate())

	require.True(t, c.IsCacheConfigured())
	require.True(t, c.IsClientCredentialsConfigured())
	require.True(t, c.IsIntrospectionEnabled())
	require.True(t, c.IsValidationConfigured())
	require.True(t, c.IsJWKSConfigured())
	require.True(t, c.IsPresetsConfigured())
	require.True(t, c.IsRevocationConfigured())
}

func TestOIDC_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mut   func(*authconfig.OIDC)
		field string
	}{
		{"missing discovery url", func(c *authconfig.OIDC) { c.DiscoveryUrl = "" }, "DiscoveryUrl"},
		{"invalid discovery url", func(c *authconfig.OIDC) { c.DiscoveryUrl = "::bad" }, "DiscoveryUrl"},
		{"clock skew too small", func(c *authconfig.OIDC) { c.ClockSkew = time.Millisecond }, "ClockSkew"},
		{"clock skew too large", func(c *authconfig.OIDC) { c.ClockSkew = time.Hour }, "ClockSkew"},
		{"introspection without credentials", func(c *authconfig.OIDC) {
			c.Introspection = &authconfig.OIDCIntrospection{Enabled: true}
		}, "Introspection"},
		{"invalid cache", func(c *authconfig.OIDC) { c.Cache = &authconfig.OIDCCache{Enabled: true} }, "Cache"},
		{"invalid credentials", func(c *authconfig.OIDC) { c.ClientCredentials = &authconfig.OIDCClientCredentials{} }, "ClientCredentials"},
		{"invalid jwks", func(c *authconfig.OIDC) { c.JWKS = &authconfig.OIDCJwks{RefreshEnabled: true} }, "JWKS"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := validOIDC()
			tc.mut(&c)
			require.Contains(t, fieldErrors(t, c.Validate()), tc.field)
		})
	}

	c := validOIDC()
	c.Introspection = &authconfig.OIDCIntrospection{Enabled: true}
	require.False(t, c.IsIntrospectionEnabled(), "introspection needs client credentials")
}

func TestOIDCCache_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&authconfig.OIDCCache{}).Validate(), "disabled cache needs no prefixes")
	cache := authconfig.DefaultOIDCCache()
	cache.Enabled = true
	require.NoError(t, cache.Validate())

	verrs := fieldErrors(t, (&authconfig.OIDCCache{Enabled: true}).Validate())
	require.Contains(t, verrs, "TokensKeyPrefix")
	require.Contains(t, verrs, "RevokedTokensKeyPrefix")
}

func TestOIDCExpression_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&authconfig.OIDCExpression{Expression: "claims.sub != ''"}).Validate())
	require.Contains(t, fieldErrors(t, (&authconfig.OIDCExpression{}).Validate()), "Expression")
}

func TestOIDCValidation_Validate(t *testing.T) {
	t.Parallel()

	v := authconfig.OIDCValidation{}
	require.NoError(t, v.Validate())
	require.False(t, v.IsClaimsConfigured())

	v.Claims = &authconfig.OIDCClaims{Audience: []string{"api"}}
	require.True(t, v.IsClaimsConfigured())
	require.NoError(t, v.Validate())

	verrs := fieldErrors(t, (&authconfig.OIDCValidation{
		Issuer:           new("not a url"),
		Claims:           &authconfig.OIDCClaims{Audience: []string{""}},
		MaxTokenLifetime: -time.Second,
	}).Validate())
	for _, f := range []string{"Issuer", "Claims", "MaxTokenLifetime"} {
		require.Contains(t, verrs, f)
	}
}

func TestOIDCClaims_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&authconfig.OIDCClaims{
		Required: []string{"sub"}, Expected: map[string]string{"tenant": "a"},
		RequireAuthorizedParty: true, AllowedAuthorizedParties: []string{"web"},
	}).Validate())

	verrs := fieldErrors(t, (&authconfig.OIDCClaims{
		Required:               []string{""},
		AllowedClientIds:       []string{""},
		RequiredScopes:         []string{""},
		Ignored:                []string{""},
		Expected:               map[string]string{"tenant": ""},
		RequireAuthorizedParty: true,
	}).Validate())
	for _, f := range []string{"Required", "AllowedClientIds", "RequiredScopes", "Ignored", "Expected", "AllowedAuthorizedParties"} {
		require.Contains(t, verrs, f)
	}
}

func TestOIDCJwks_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&authconfig.OIDCJwks{}).Validate(), "refresh disabled needs no schedule")
	require.Contains(t, fieldErrors(t, (&authconfig.OIDCJwks{HTTPTimeout: -1}).Validate()), "HTTPTimeout")
}

func TestOIDCPresets_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&authconfig.OIDCPresets{}).Validate())

	preset := authconfig.OIDCPreset{Name: "partner-a", Issuer: new("https://idp.partner.example")}
	require.False(t, preset.IsClaimsConfigured())
	selector := authconfig.OIDCSelector{Expression: "claims.iss == 'x'", PresetName: "partner-a"}
	require.NoError(t, (&authconfig.OIDCPresets{
		List: []authconfig.OIDCPreset{preset}, Selectors: []authconfig.OIDCSelector{selector},
	}).Validate())

	err := (&authconfig.OIDCPresets{List: []authconfig.OIDCPreset{preset}}).Validate()
	require.Contains(t, fieldErrors(t, err), "presets", "list without selectors")
}

func TestOIDCPreset_Validate(t *testing.T) {
	t.Parallel()

	p := authconfig.OIDCPreset{Name: "a", Claims: &authconfig.OIDCClaims{}}
	require.True(t, p.IsClaimsConfigured())
	require.NoError(t, p.Validate())

	verrs := fieldErrors(t, (&authconfig.OIDCPreset{
		Issuer: new("::bad"), MaxTokenLifetime: -time.Second, Claims: &authconfig.OIDCClaims{Required: []string{""}},
	}).Validate())
	for _, f := range []string{"Name", "Issuer", "MaxTokenLifetime", "Claims"} {
		require.Contains(t, verrs, f)
	}

	verrs = fieldErrors(t, (&authconfig.OIDCSelector{}).Validate())
	require.Contains(t, verrs, "Expression")
	require.Contains(t, verrs, "PresetName")
}

func TestOIDCRevocation_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, validRevocation().Validate())

	r := validRevocation()
	r.SyncSchedule = ""
	r.ItemType = "sub"
	r.Filter = nil
	r.Source = nil
	verrs := fieldErrors(t, r.Validate())
	for _, f := range []string{"SyncSchedule", "ItemType", "Filter", "Source"} {
		require.Contains(t, verrs, f)
	}

	r = validRevocation()
	require.Equal(t, 3*time.Hour, r.MaxStaleness)
	r.MaxStaleness = 0
	require.NoError(t, r.Validate(), "zero disables the bound")
	r.MaxStaleness = -time.Second
	require.Contains(t, fieldErrors(t, r.Validate()), "MaxStaleness")

	require.NoError(t, (&authconfig.OIDCRevocationSource{File: "/etc/revoked.txt"}).Validate())
	verrs = fieldErrors(t, (&authconfig.OIDCRevocationSource{}).Validate())
	require.Contains(t, verrs, "URL")
	require.Contains(t, verrs, "File")
	require.Contains(t, fieldErrors(t, (&authconfig.OIDCRevocationSource{URL: "::bad"}).Validate()), "URL")
}

func TestOAuth2Client_Defaults(t *testing.T) {
	t.Parallel()

	c := authconfig.DefaultOAuth2Client()
	c.ClientId = "svc"
	c.TokenUrl = "https://idp.example.com/token"
	require.NoError(t, c.Validate())
	require.False(t, c.IsClientAuthConfigured())
	require.False(t, c.IsRetryConfigured())
	require.False(t, c.IsDiscovery())

	retry := authconfig.DefaultOAuth2ClientRetry()
	require.NoError(t, retry.Validate())
	c.Retry = &retry
	require.True(t, c.IsRetryConfigured())
	require.NoError(t, c.Validate())

	verrs := fieldErrors(t, (&authconfig.OAuth2ClientRetry{Attempts: -1, BaseDelay: -1, MaxDelay: -1}).Validate())
	for _, f := range []string{"Attempts", "BaseDelay", "MaxDelay"} {
		require.Contains(t, verrs, f)
	}
}

func TestOPA_IsEnabled(t *testing.T) {
	t.Parallel()

	var nilCfg *authconfig.OPA
	require.False(t, nilCfg.IsEnabled())

	gitlab := authconfig.DefaultOPAGitLab()
	require.Equal(t, "main", gitlab.Ref)

	tests := []struct {
		name string
		cfg  authconfig.OPA
		want bool
	}{
		{"gitlab configured", authconfig.OPA{Source: authconfig.OPASourceGitLab, GitLab: &gitlab}, true},
		{"gitlab missing", authconfig.OPA{Source: authconfig.OPASourceGitLab}, false},
		{"s3 configured", authconfig.OPA{Source: authconfig.OPASourceS3, S3: &authconfig.OPAS3{}}, true},
		{"s3 missing", authconfig.OPA{Source: authconfig.OPASourceS3}, false},
		{"embed", authconfig.OPA{Source: authconfig.OPASourceEmbed}, true},
		{"filesystem with bundle", authconfig.OPA{Source: authconfig.OPASourceFilesystem, BundlePath: "/policies"}, true},
		{"filesystem without bundle", authconfig.OPA{Source: authconfig.OPASourceFilesystem}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.cfg.IsEnabled())
		})
	}
}

func TestOPA_Validate(t *testing.T) {
	t.Parallel()

	c := authconfig.DefaultOPA()
	c.BundlePath = "/policies"
	require.NoError(t, c.Validate())

	tests := []struct {
		name  string
		cfg   authconfig.OPA
		field string
	}{
		{"missing source", authconfig.OPA{Query: "q"}, "Source"},
		{"unknown source", authconfig.OPA{Source: "http", Query: "q"}, "Source"},
		{"filesystem without bundle", authconfig.OPA{Source: authconfig.OPASourceFilesystem, Query: "q"}, "BundlePath"},
		{"gitlab without section", authconfig.OPA{Source: authconfig.OPASourceGitLab, Query: "q"}, "GitLab"},
		{"s3 without section", authconfig.OPA{Source: authconfig.OPASourceS3, Query: "q"}, "S3"},
		{"missing query", authconfig.OPA{Source: authconfig.OPASourceEmbed}, "Query"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, fieldErrors(t, tc.cfg.Validate()), tc.field)
		})
	}
}

func TestSPIFFE(t *testing.T) {
	t.Parallel()

	var nilCfg *authconfig.SPIFFE
	require.False(t, nilCfg.IsEnabled())

	s := authconfig.DefaultSPIFFE()
	require.True(t, s.IsEnabled())
	require.Contains(t, fieldErrors(t, s.Validate()), "AllowedTrustDomains")

	s.AllowedIDs = []string{"spiffe://example.org/svc"}
	require.NoError(t, s.Validate())

	require.NoError(t, (&authconfig.SPIFFE{AllowedTrustDomains: []string{"example.org"}}).Validate())
}

// TestOIDCPresets_ValidatesEntries pins that List and Selectors entries,
// held by value with pointer-receiver Validate, are validated.
func TestOIDCPresets_ValidatesEntries(t *testing.T) {
	t.Parallel()

	good := authconfig.OIDCPreset{Name: "partner-a"}
	sel := authconfig.OIDCSelector{Expression: "true", PresetName: "partner-a"}

	err := (&authconfig.OIDCPresets{
		List: []authconfig.OIDCPreset{good, {Issuer: new("::bad")}}, Selectors: []authconfig.OIDCSelector{sel},
	}).Validate()
	require.Contains(t, fieldErrors(t, err), "List")

	err = (&authconfig.OIDCPresets{
		List: []authconfig.OIDCPreset{good}, Selectors: []authconfig.OIDCSelector{sel, {Priority: 1}},
	}).Validate()
	require.Contains(t, fieldErrors(t, err), "Selectors")
}

// TestOIDCExpression_IsValidated pins that the optional CEL expression of a
// preset and of the base validation block is validated when set.
func TestOIDCExpression_IsValidated(t *testing.T) {
	t.Parallel()

	empty := &authconfig.OIDCExpression{Name: "no-expression"}
	require.Contains(t, fieldErrors(t, (&authconfig.OIDCPreset{Name: "a", Expression: empty}).Validate()), "Expression")
	require.Contains(t, fieldErrors(t, (&authconfig.OIDCValidation{Expression: empty}).Validate()), "Expression")

	set := &authconfig.OIDCExpression{Expression: "claims.sub != ''"}
	require.NoError(t, (&authconfig.OIDCPreset{Name: "a", Expression: set}).Validate())
	require.NoError(t, (&authconfig.OIDCValidation{Expression: set}).Validate())
}
