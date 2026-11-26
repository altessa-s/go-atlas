// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/auth/oidc/rules"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

// coverageIdP is a minimal httptest OIDC issuer with a one-key JWKS.
type coverageIdP struct {
	srv     *httptest.Server
	key     *rsa.PrivateKey
	rotated atomic.Pointer[rsa.PrivateKey]
	down    atomic.Bool // makes the JWKS endpoint fail
}

func newCoverageIdP(t *testing.T) *coverageIdP {
	t.Helper()
	idp := &coverageIdP{key: testhelpers.GenerateRSAKey(t, 2048)}
	mux := http.NewServeMux()
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.srv.URL,
			"jwks_uri":                              idp.srv.URL + "/jwks",
			"introspection_endpoint":                idp.srv.URL + "/introspect",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		if idp.down.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		key, kid := idp.key, "k1"
		if rotated := idp.rotated.Load(); rotated != nil {
			key, kid = rotated, "k2"
		}
		pub := key.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/introspect", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"active": false})
	})
	return idp
}

func (idp *coverageIdP) discoveryURL() string {
	return idp.srv.URL + "/.well-known/openid-configuration"
}

func (idp *coverageIdP) sign(t *testing.T, overrides map[string]any) string {
	t.Helper()
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": idp.srv.URL, "sub": "user-1", "aud": "svc",
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}
	maps.Copy(claims, overrides)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	key, kid := idp.key, "k1"
	if rotated := idp.rotated.Load(); rotated != nil {
		key, kid = rotated, "k2"
	}
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(key)
	require.NoError(t, err)
	return raw
}

// provider builds the factory's options from cfg and starts a provider
// against the test IdP, swapping in a loopback-capable HTTP client.
func (idp *coverageIdP) provider(t *testing.T, b *ProviderBuilder) *oidc.Provider {
	t.Helper()
	opts, err := b.buildProviderOptions(t.Context())
	require.NoError(t, err)
	opts = append(opts,
		oidc.WithDiscoveryValidationMode(oidc.DiscoveryValidationModeDisabled),
		oidc.WithHTTPClient(idp.srv.Client()))
	p, err := oidc.NewProvider(t.Context(), idp.discoveryURL(), opts...)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

// mapCacher is an in-memory oidc.Cacher counting writes.
type mapCacher struct {
	mu    sync.Mutex
	items map[string][]byte
	saves int
}

var errMiss = errors.New("miss")

func (c *mapCacher) Save(_ context.Context, key string, value any, _ ...time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = b
	c.saves++
	return nil
}

func (c *mapCacher) Get(_ context.Context, key string, value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.items[key]
	if !ok {
		return errMiss
	}
	return json.Unmarshal(b, value)
}

func (c *mapCacher) saveCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saves
}

func TestProviderBuilder_ValidationConfigIsEnforced(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	issuer := idp.srv.URL
	cfg := &authconfig.OIDC{
		DiscoveryUrl: idp.discoveryURL(),
		ClockSkew:    time.Second,
		JWKS:         &authconfig.OIDCJwks{HTTPTimeout: 5 * time.Second},
		Validation: &authconfig.OIDCValidation{
			Issuer:           &issuer,
			MaxTokenLifetime: 2 * time.Hour,
			Expression:       &authconfig.OIDCExpression{Expression: `claims.tier == "gold"`},
			Claims: &authconfig.OIDCClaims{
				Audience: []string{"svc"},
				Required: []string{"tier"},
				Expected: map[string]string{"team": "core"},
			},
		},
	}
	p := idp.provider(t, New(cfg).UseLogger(slog.New(slog.DiscardHandler)))

	claims, err := p.ValidateToken(t.Context(), idp.sign(t, map[string]any{"tier": "gold", "team": "core"}))
	require.NoError(t, err)
	require.Equal(t, "user-1", claims["sub"])

	tests := []struct {
		name      string
		overrides map[string]any
	}{
		{name: "wrong audience", overrides: map[string]any{"tier": "gold", "team": "core", "aud": "other"}},
		{name: "expected claim mismatch", overrides: map[string]any{"tier": "gold", "team": "edge"}},
		{name: "cel expression false", overrides: map[string]any{"tier": "silver", "team": "core"}},
		{name: "wrong issuer", overrides: map[string]any{"tier": "gold", "team": "core", "iss": "https://evil.example"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := p.ValidateToken(t.Context(), idp.sign(t, tc.overrides))
			require.Error(t, err)
		})
	}
}

func TestProviderBuilder_PresetsAreSelectable(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	cfg := &authconfig.OIDC{
		DiscoveryUrl: idp.discoveryURL(),
		Presets: &authconfig.OIDCPresets{
			List: []authconfig.OIDCPreset{{
				Name:       "admin",
				Expression: &authconfig.OIDCExpression{Name: "is-admin", Expression: `claims.role == "admin"`},
				Claims:     &authconfig.OIDCClaims{Audience: []string{"svc"}},
			}},
			Selectors: []authconfig.OIDCSelector{{Expression: `claims.role == "admin"`, PresetName: "admin", Priority: 1}},
		},
	}
	p := idp.provider(t, New(cfg).UseDefaultLogger())

	_, err := p.ValidateTokenWithPreset(t.Context(), idp.sign(t, map[string]any{"role": "admin"}), "admin")
	require.NoError(t, err)

	_, err = p.ValidateTokenWithPreset(t.Context(), idp.sign(t, map[string]any{"role": "user"}), "admin")
	require.Error(t, err, "the preset's CEL rule must reject a non-admin token")
}

func TestProviderBuilder_TokenCacheWiredWhenEnabled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		enabled   bool
		wantSaves bool
	}{
		{name: "enabled", enabled: true, wantSaves: true},
		{name: "disabled", enabled: false, wantSaves: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			idp := newCoverageIdP(t)
			cache := &mapCacher{items: map[string][]byte{}}
			cfg := &authconfig.OIDC{
				DiscoveryUrl: idp.discoveryURL(),
				Cache:        &authconfig.OIDCCache{Enabled: tc.enabled, TokensKeyPrefix: "t:", RevokedTokensKeyPrefix: "r:"},
				Validation:   &authconfig.OIDCValidation{Claims: &authconfig.OIDCClaims{Audience: []string{"svc"}}},
			}
			p := idp.provider(t, New(cfg).UseTokenCache(cache))

			_, err := p.ValidateToken(t.Context(), idp.sign(t, nil))
			require.NoError(t, err)
			require.Equal(t, tc.wantSaves, cache.saveCount() > 0)
		})
	}
}

func TestProviderBuilder_IntrospectionRejectsInactiveToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		enabled bool
		wantErr bool
	}{
		{name: "introspection enabled", enabled: true, wantErr: true},
		{name: "introspection disabled", enabled: false, wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			idp := newCoverageIdP(t)
			cfg := &authconfig.OIDC{
				DiscoveryUrl:      idp.discoveryURL(),
				Validation:        &authconfig.OIDCValidation{Claims: &authconfig.OIDCClaims{Audience: []string{"svc"}}},
				ClientCredentials: &authconfig.OIDCClientCredentials{ClientId: "id", ClientSecret: "secret"},
				Introspection:     &authconfig.OIDCIntrospection{Enabled: tc.enabled, FailOpen: true},
			}
			p := idp.provider(t, New(cfg))

			_, err := p.ValidateToken(t.Context(), idp.sign(t, nil))
			if tc.wantErr {
				require.Error(t, err, "the IdP reports the token inactive")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestProviderBuilder_BuildRejectsLoopbackDiscovery(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	// The builder's own HTTP client is SSRF-protected, so a loopback IdP is
	// refused at discovery time.
	_, err := New(&authconfig.OIDC{DiscoveryUrl: idp.discoveryURL()}).Build(t.Context())
	require.Error(t, err)
}

func TestValidationOptionsFromConfig_NilConfig(t *testing.T) {
	t.Parallel()
	_, err := validationOptionsFromConfig(nil, time.Second)
	require.Error(t, err)
}

func TestPresetFromConfig_NilConfig(t *testing.T) {
	t.Parallel()
	_, err := presetFromConfig(nil, time.Second)
	require.Error(t, err)
}

func TestBuildCELValidationRule_EmptyExpression(t *testing.T) {
	t.Parallel()
	require.Nil(t, buildCELValidationRule(nil, "x"))
	require.Nil(t, buildCELValidationRule(&authconfig.OIDCExpression{}, "x"))
	require.NotNil(t, buildCELValidationRule(&authconfig.OIDCExpression{Expression: "true"}, "x"))
}

func TestBuildClaimsValidationOptions_Nil(t *testing.T) {
	t.Parallel()
	opts, err := buildClaimsValidationOptions(nil)
	require.NoError(t, err)
	require.Nil(t, opts)
}

func TestProviderBuilderMinimumLifetimeYAML(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	doc := fmt.Sprintf(`oidc:
  discoveryURL: %s
  validation:
    minTokenLifetime: 90m
    maxTokenLifetime: 2h
    claims:
      audience: [svc]
  presets:
    list:
      - name: selected
        minTokenLifetime: 30m
        maxTokenLifetime: 2h
        claims:
          audience: [svc]
    selectors:
      - presetName: selected
        expression: claims.sub == "selected"
`, idp.discoveryURL())
	for _, tc := range []struct {
		name, old, replacement string
		want                   error
	}{
		{name: "valid"},
		{"negative default minimum", "90m", "-1s", loader.ErrValidation},
		{"preset minimum above maximum", "30m", "3h", loader.ErrValidation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "auth.yaml")
			require.NoError(t, os.WriteFile(path, []byte(strings.Replace(doc, tc.old, tc.replacement, 1)), 0o600))
			var cfg authconfig.Config
			_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(&cfg)
			require.ErrorIs(t, err, tc.want)
			if tc.want != nil {
				return
			}

			p := idp.provider(t, New(cfg.OIDC))
			now := time.Now()
			for _, tc := range []struct {
				name, subject string
				ttl           time.Duration
				want          error
			}{
				{"default rejects one hour", "user-1", time.Hour, oidc.ErrTokenInvalid},
				{"preset accepts one hour", "selected", time.Hour, nil},
				{"preset rejects fifteen minutes", "selected", 15 * time.Minute, oidc.ErrTokenInvalid},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					token := idp.sign(t, map[string]any{"sub": tc.subject, "iat": now.Unix(), "exp": now.Add(tc.ttl).Unix()})
					_, err := p.ValidateToken(t.Context(), token)
					require.ErrorIs(t, err, tc.want)
				})
			}
		})
	}
}

func TestProviderBuilderRefreshesRotatedKeyWithoutJWKSConfig(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	p := idp.provider(t, New(&authconfig.OIDC{DiscoveryUrl: idp.discoveryURL()}))
	_, err := p.ValidateTokenWithOptions(t.Context(), idp.sign(t, nil), oidc.WithValidationAudience("svc"))
	require.NoError(t, err)
	idp.rotated.Store(testhelpers.GenerateRSAKey(t, 2048))
	_, err = p.ValidateTokenWithOptions(t.Context(), idp.sign(t, nil), oidc.WithValidationAudience("svc"))
	require.NoError(t, err)
}

func TestProviderBuilderEnforcesAuthorizedPartyAllowlist(t *testing.T) {
	t.Parallel()
	idp := newCoverageIdP(t)
	p := idp.provider(t, New(&authconfig.OIDC{
		DiscoveryUrl: idp.discoveryURL(),
		Validation: &authconfig.OIDCValidation{Claims: &authconfig.OIDCClaims{
			Audience: []string{"svc"}, AllowedAuthorizedParties: []string{"web"},
		}},
	}))
	_, err := p.ValidateToken(t.Context(), idp.sign(t, map[string]any{"azp": "web"}))
	require.NoError(t, err)
	_, err = p.ValidateToken(t.Context(), idp.sign(t, map[string]any{"azp": "evil"}))
	require.ErrorIs(t, err, oidc.ErrTokenInvalid)
}

func TestProviderBuilderSchedulesDefaultJWKSRefresh(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		const issuer = "https://idp.example"
		key := testhelpers.GenerateRSAKey(t, 2048)
		jwks, err := json.Marshal(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": "active", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
		require.NoError(t, err)
		var fetches atomic.Int32
		client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/jwks" {
				fetches.Add(1)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(jwks)))}, nil
			}
			metadata := `{"issuer":"` + issuer + `","jwks_uri":"` + issuer + `/jwks","id_token_signing_alg_values_supported":["RS256"]}`
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(metadata))}, nil
		})}
		discovery := issuer + "/.well-known/openid-configuration"
		opts, err := New(&authconfig.OIDC{DiscoveryUrl: discovery}).buildProviderOptions(t.Context())
		require.NoError(t, err)
		p, err := oidc.NewProvider(t.Context(), discovery, append(opts, oidc.WithHTTPClient(client))...)
		require.NoError(t, err)
		defer p.Close()
		require.EqualValues(t, 1, fetches.Load())
		time.Sleep(2 * time.Hour)
		synctest.Wait()
		require.EqualValues(t, 3, fetches.Load())
	})
}

func TestBlankAuthorizedPartiesRejected(t *testing.T) {
	t.Parallel()
	parties := []string{"web", " \t"}
	claims := &authconfig.OIDCClaims{AllowedAuthorizedParties: parties}
	policy := &authconfig.ServiceConfig{
		Version:           authconfig.SupportedConfigVersion,
		DefaultValidation: &authconfig.ValidationRulesConfig{AllowedAuthorizedParties: parties},
	}
	t.Run("claims config", func(t *testing.T) {
		require.Error(t, claims.Validate())
	})
	t.Run("claims factory", func(t *testing.T) {
		cfg := &authconfig.OIDC{Validation: &authconfig.OIDCValidation{Claims: claims}}
		_, err := New(cfg).buildProviderOptions(t.Context())
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	})
	t.Run("claims preset", func(t *testing.T) {
		_, err := presetFromConfig(&authconfig.OIDCPreset{Claims: claims}, 0)
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	})
	t.Run("service config", func(t *testing.T) {
		require.ErrorIs(t, authconfig.ValidateServiceConfig(policy), rules.ErrInvalidConfig)
	})
	t.Run("service factory", func(t *testing.T) {
		_, err := ServiceConfigOptions(policy)
		require.ErrorIs(t, err, rules.ErrInvalidConfig)
	})
}
