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
	"log/slog"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

// coverageIdP is a minimal httptest OIDC issuer with a one-key JWKS.
type coverageIdP struct {
	srv *httptest.Server
	key *rsa.PrivateKey
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
		w.Header().Set("Content-Type", "application/json")
		pub := idp.key.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig",
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
	tok.Header["kid"] = "k1"
	raw, err := tok.SignedString(idp.key)
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
	require.Nil(t, buildClaimsValidationOptions(nil))
}
