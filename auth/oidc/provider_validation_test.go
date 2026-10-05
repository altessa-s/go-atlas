// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"iter"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// testAudience is the audience every test token carries and every test
// provider expects.
const testAudience = "svc"

// testIdP is an httptest OIDC issuer serving discovery, a one-key JWKS, an
// RFC 7662 introspection endpoint and a plain-text revocation list.
type testIdP struct {
	srv            *httptest.Server
	keyMu          sync.Mutex
	key            *rsa.PrivateKey
	kid            string
	active         atomic.Bool
	introspections atomic.Int32
	revokedStatus  atomic.Int32
	jwksStatus     atomic.Int32
	mu             sync.Mutex
	revoked        []string
}

func newTestIdP(t testing.TB) *testIdP {
	t.Helper()
	idp := &testIdP{key: testhelpers.GenerateRSAKey(t, 2048), kid: "k1"}
	idp.active.Store(true)
	idp.revokedStatus.Store(http.StatusOK)
	idp.jwksStatus.Store(http.StatusOK)

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
		if status := int(idp.jwksStatus.Load()); status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		key, kid := idp.signingKey()
		pub := key.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/introspect", func(w http.ResponseWriter, _ *http.Request) {
		idp.introspections.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"active": idp.active.Load()})
	})
	mux.HandleFunc("/revoked", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(idp.revokedStatus.Load()))
		idp.mu.Lock()
		defer idp.mu.Unlock()
		_, _ = w.Write([]byte(strings.Join(idp.revoked, "\n")))
	})
	return idp
}

func (idp *testIdP) signingKey() (*rsa.PrivateKey, string) {
	idp.keyMu.Lock()
	defer idp.keyMu.Unlock()
	return idp.key, idp.kid
}

// rotate replaces the IdP's only signing key; the JWKS then serves just the
// new key under kid.
func (idp *testIdP) rotate(t testing.TB, kid string) {
	t.Helper()
	key := testhelpers.GenerateRSAKey(t, 2048)
	idp.keyMu.Lock()
	defer idp.keyMu.Unlock()
	idp.key, idp.kid = key, kid
}

func (idp *testIdP) discoveryURL() string { return idp.srv.URL + "/.well-known/openid-configuration" }

func (idp *testIdP) setRevoked(items ...string) {
	idp.mu.Lock()
	defer idp.mu.Unlock()
	idp.revoked = items
}

// claims returns a valid claim set issued by idp; overrides replace or, with
// a nil value, delete individual claims.
func (idp *testIdP) claims(overrides map[string]any) jwt.MapClaims {
	now := time.Now()
	c := jwt.MapClaims{
		"iss": idp.srv.URL, "sub": "user-1", "aud": testAudience,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}
	for k, v := range overrides {
		if v == nil {
			delete(c, k)
			continue
		}
		c[k] = v
	}
	return c
}

func (idp *testIdP) sign(t testing.TB, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	key, kid := idp.signingKey()
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(key)
	require.NoError(t, err)
	return raw
}

func (idp *testIdP) newProvider(t testing.TB, opts ...Option) *Provider {
	t.Helper()
	p, err := idp.tryNewProvider(t, opts...)
	require.NoError(t, err)
	return p
}

func (idp *testIdP) tryNewProvider(t testing.TB, opts ...Option) (*Provider, error) {
	t.Helper()
	base := []Option{
		WithLogger(slog.New(slog.DiscardHandler)),
		WithHTTPClientOptions(httpclient.WithRetryMax(0), httpclient.WithoutProxy()),
		// httptest serves plain HTTP on loopback.
		WithDiscoveryValidationMode(DiscoveryValidationModeDisabled),
		WithDefaultValidationOptions(WithValidationAudience(testAudience)),
	}
	p, err := NewProvider(t.Context(), idp.discoveryURL(), append(base, opts...)...)
	if err == nil {
		t.Cleanup(p.Close)
	}
	return p, err
}

// memCacher is a JSON-serializing Cacher (like data/cache) that records the
// TTL of every Save.
type memCacher struct {
	mu    sync.Mutex
	items map[string][]byte
	ttls  []time.Duration
}

var errCacheMiss = errors.New("cache miss")

func newMemCacher() *memCacher { return &memCacher{items: map[string][]byte{}} }

func (c *memCacher) Save(_ context.Context, key string, value any, ttl ...time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = b
	var d time.Duration
	if len(ttl) > 0 {
		d = ttl[0]
	}
	c.ttls = append(c.ttls, d)
	return nil
}

func (c *memCacher) Get(_ context.Context, key string, value any) error {
	c.mu.Lock()
	b, ok := c.items[key]
	c.mu.Unlock()
	if !ok {
		return errCacheMiss
	}
	return json.Unmarshal(b, value)
}

func (c *memCacher) saves() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.ttls...)
}

func TestValidate_CacheHitStillEnforcesRequestedPolicy(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	cache := newMemCacher()
	strict := NewValidationPreset("strict",
		WithValidationAudience(testAudience), WithValidationRequiredScopes("admin"))
	p := idp.newProvider(t, WithTokenCache(cache), WithPresets(strict))

	raw := idp.sign(t, idp.claims(map[string]any{"scope": "read"}))

	_, err := p.ValidateToken(t.Context(), raw)
	require.NoError(t, err)
	require.Len(t, cache.saves(), 1, "the signature verification must be cached")

	_, err = p.ValidateTokenWithPreset(t.Context(), raw, "strict")
	require.ErrorIs(t, err, ErrTokenInvalid, "a cached token must still satisfy the requested preset")

	_, err = p.ValidateTokenWithOptions(t.Context(), raw,
		WithValidationAudience(testAudience), WithValidationRequiredScopes("admin"))
	require.ErrorIs(t, err, ErrTokenInvalid, "a cached token must still satisfy per-call options")

	_, err = p.ValidateTokenWithOptions(t.Context(), raw, WithValidationAudience("other"))
	require.ErrorIs(t, err, ErrTokenInvalid, "a cached token must still match the requested audience")

	claims, err := p.ValidateToken(t.Context(), raw)
	require.NoError(t, err)
	require.Equal(t, "user-1", claims["sub"])
	require.Len(t, cache.saves(), 1, "hits must not re-save the entry")
}

func TestValidate_CacheHitReappliesAlgorithmAllowList(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	cache := newMemCacher()
	p := idp.newProvider(t, WithTokenCache(cache))
	raw := idp.sign(t, idp.claims(nil))

	_, err := p.ValidateToken(t.Context(), raw)
	require.NoError(t, err)

	_, err = p.ValidateTokenWithOptions(t.Context(), raw,
		WithValidationAudience(testAudience), WithValidationValidMethods("ES256"))
	require.ErrorIs(t, err, ErrTokenInvalid)
}

func TestValidate_NeverCachesWithNonPositiveTTL(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	cache := newMemCacher()
	p := idp.newProvider(t, WithTokenCache(cache))

	// Past exp but inside the default 30s leeway: valid, but must not be cached
	// (a zero TTL means "no expiration" to data/cache).
	expired := idp.sign(t, idp.claims(map[string]any{"exp": time.Now().Add(-5 * time.Second).Unix()}))
	_, err := p.ValidateToken(t.Context(), expired)
	require.NoError(t, err)

	noExp := idp.sign(t, idp.claims(map[string]any{"exp": nil}))
	_, err = p.ValidateTokenWithOptions(t.Context(), noExp,
		WithValidationAudience(testAudience), WithValidationRequiredClaims("sub"))
	require.NoError(t, err)

	require.Empty(t, cache.saves())

	valid := idp.sign(t, idp.claims(nil))
	_, err = p.ValidateToken(t.Context(), valid)
	require.NoError(t, err)
	saves := cache.saves()
	require.Len(t, saves, 1)
	require.Positive(t, saves[0])
	require.LessOrEqual(t, saves[0], time.Hour)
}

func TestValidate_CacheHitReevaluatesTimeClaims(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	cache := newMemCacher()
	p := idp.newProvider(t, WithTokenCache(cache))
	header := map[string]any{"alg": "RS256", "kid": "k1"}

	tests := []struct {
		name      string
		overrides map[string]any
	}{
		{name: "expired beyond leeway", overrides: map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}},
		{name: "not yet valid", overrides: map[string]any{"nbf": time.Now().Add(time.Hour).Unix()}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := "cached-" + strings.ReplaceAll(tc.name, " ", "-")
			entry := cachedToken{
				Claims: idp.claims(tc.overrides), Header: header,
				KeyFingerprint: p.currentKeyFingerprint(t.Context(), header),
			}
			require.NotEmpty(t, entry.KeyFingerprint)
			require.NoError(t, cache.Save(t.Context(), p.signatureCacheKey(raw), entry, time.Hour))

			_, err := p.ValidateToken(t.Context(), raw)
			require.ErrorIs(t, err, ErrTokenInvalid)
		})
	}
}

func TestSignatureCacheKey_BoundToTrustDomainAndVersioned(t *testing.T) {
	t.Parallel()

	idpA, idpB := newTestIdP(t), newTestIdP(t)
	cache := newMemCacher()
	pA := idpA.newProvider(t, WithTokenCache(cache))
	pB := idpB.newProvider(t, WithTokenCache(cache))

	// Signed by A but claiming B's issuer: A rejects it on issuer after
	// caching the signature verification.
	raw := idpA.sign(t, idpA.claims(map[string]any{"iss": idpB.srv.URL}))
	_, err := pA.ValidateToken(t.Context(), raw)
	require.ErrorIs(t, err, ErrTokenInvalid)
	require.Len(t, cache.saves(), 1)

	_, err = pB.ValidateToken(t.Context(), raw)
	require.ErrorIs(t, err, ErrTokenInvalid, "B must never trust A's cached signature verification")

	require.NotEqual(t, pA.signatureCacheKey(raw), pB.signatureCacheKey(raw))
	require.NotEqual(t, tokenCacheKey(DefaultTokensCacheKeyPrefix, raw), pA.signatureCacheKey(raw),
		"the signature cache must not share keys with the legacy claims cache")
}

func TestPreset_BindsDiscoveryIssuer(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	preset := NewValidationPreset("svc", WithValidationAudience(testAudience))
	p := idp.newProvider(t, WithPresets(preset), WithPresetRules(PresetRule{
		Priority:   1,
		Matcher:    func(map[string]any) bool { return true },
		PresetName: "svc",
	}))

	ok := idp.sign(t, idp.claims(nil))
	_, err := p.ValidateTokenWithPreset(t.Context(), ok, "svc")
	require.NoError(t, err)

	for name, iss := range map[string]any{"wrong issuer": "https://evil.example", "missing issuer": nil} {
		raw := idp.sign(t, idp.claims(map[string]any{"iss": iss}))

		_, err := p.ValidateTokenWithPreset(t.Context(), raw, "svc")
		require.Error(t, err, "explicit preset, %s", name)

		_, err = p.ValidateToken(t.Context(), raw)
		require.Error(t, err, "auto-selected preset, %s", name)
	}
}

func TestPreset_SharedPresetValueStaysProviderBound(t *testing.T) {
	t.Parallel()

	idpA, idpB := newTestIdP(t), newTestIdP(t)
	shared := NewValidationPreset("svc", WithValidationAudience(testAudience))
	pA := idpA.newProvider(t, WithPresets(shared))
	_ = idpB.newProvider(t, WithPresets(shared))

	// Signed by A (so A's signature check passes) but claiming B's issuer.
	raw := idpA.sign(t, idpA.claims(map[string]any{"iss": idpB.srv.URL}))
	_, err := pA.ValidateTokenWithPreset(t.Context(), raw, "svc")
	require.ErrorIs(t, err, ErrTokenInvalid, "building provider B must not rebind provider A's preset to B's issuer")
}

func TestIntrospection_RunsOnlyAfterSignatureVerification(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	foreign := newTestIdP(t)
	p := idp.newProvider(t, WithIntrospection("client", "secret"))

	_, err := p.ValidateToken(t.Context(), "not-a-jwt")
	require.ErrorIs(t, err, ErrTokenInvalid)

	_, err = p.ValidateToken(t.Context(), foreign.sign(t, idp.claims(nil)))
	require.ErrorIs(t, err, ErrTokenInvalid)
	require.Zero(t, idp.introspections.Load(), "unverified tokens must never reach the IdP")

	_, err = p.ValidateToken(t.Context(), idp.sign(t, idp.claims(nil)))
	require.NoError(t, err)
	require.EqualValues(t, 1, idp.introspections.Load())

	idp.active.Store(false)
	_, err = p.ValidateToken(t.Context(), idp.sign(t, idp.claims(map[string]any{"jti": "other"})))
	require.ErrorIs(t, err, ErrTokenRevoked)
}

func TestIntrospection_LocalStorageHitAlwaysRejects(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	raw := idp.sign(t, idp.claims(nil))
	p := idp.newProvider(t, WithIntrospection("client", "secret"),
		WithRevocationStorage(newTrackingRevocationStorage(raw)))

	_, err := p.ValidateToken(t.Context(), raw)
	require.ErrorIs(t, err, ErrTokenRevoked, "a local revocation must win over an active IdP answer")
	require.Zero(t, idp.introspections.Load())
}

func TestIntrospection_LocalStorageMissStillIntrospects(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	idp.active.Store(false)
	p := idp.newProvider(t, WithIntrospection("client", "secret"),
		WithRevocationStorage(newTrackingRevocationStorage()))

	_, err := p.ValidateToken(t.Context(), idp.sign(t, idp.claims(nil)))
	require.ErrorIs(t, err, ErrTokenRevoked, "a filter miss must not stand in for the IdP")
	require.EqualValues(t, 1, idp.introspections.Load())
}

// errRevocationStorage fails every lookup.
type errRevocationStorage struct{ err error }

func (s errRevocationStorage) IsRevoked(context.Context, string) (bool, error) { return false, s.err }
func (s errRevocationStorage) MarkRevoked(context.Context, string, time.Duration) error {
	return nil
}
func (s errRevocationStorage) Sync(context.Context) error { return nil }

func TestRevocation_StorageErrorFailMode(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	storageErr := errors.New("storage down")
	raw := idp.sign(t, idp.claims(nil))

	closed := idp.newProvider(t, WithRevocationStorage(errRevocationStorage{err: storageErr}))
	_, err := closed.ValidateToken(t.Context(), raw)
	require.ErrorIs(t, err, ErrRevocationCheck, "storage errors must fail closed by default")
	require.ErrorIs(t, err, storageErr)

	open := idp.newProvider(t, WithRevocationStorage(errRevocationStorage{err: storageErr}), WithRevocationFailOpen())
	_, err = open.ValidateToken(t.Context(), raw)
	require.NoError(t, err)
}

// rebuildableFilter is an exact in-memory Filter that also implements
// RebuildableFilter, replacing its contents from the loader.
type rebuildableFilter struct {
	mu   sync.Mutex
	data map[string]bool
}

func newRebuildableFilter() *rebuildableFilter { return &rebuildableFilter{data: map[string]bool{}} }

func (f *rebuildableFilter) MightExist(_ context.Context, v string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data[v], nil
}

func (f *rebuildableFilter) Add(_ context.Context, v string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[v] = true
	return nil
}

func (f *rebuildableFilter) AddBatch(ctx context.Context, values iter.Seq[string]) error {
	for v := range values {
		_ = f.Add(ctx, v)
	}
	return nil
}

func (f *rebuildableFilter) LastRebuild() time.Time { return time.Time{} }

func (f *rebuildableFilter) Rebuild(ctx context.Context, loader DataLoader) error {
	next := map[string]bool{}
	for v, err := range loader.StreamValues(ctx) {
		if err != nil {
			return err
		}
		next[v] = true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data = next
	return nil
}

func TestNewProvider_InitialRevocationSync(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	revoked := idp.sign(t, idp.claims(map[string]any{"jti": "revoked"}))
	valid := idp.sign(t, idp.claims(nil))
	idp.setRevoked(revoked)

	tests := []struct {
		name string
		opts func() []Option
	}{
		{name: "filter and loader options", opts: func() []Option {
			return []Option{
				WithRevocationFilter(newRebuildableFilter()),
				WithRevocationLoader(&URLRevocationLoader{URL: idp.srv.URL + "/revoked"}),
			}
		}},
		{name: "ready-made storage without client", opts: func() []Option {
			loader := &URLRevocationLoader{URL: idp.srv.URL + "/revoked"}
			return []Option{WithRevocationStorage(NewFilterRevocationStorage(newRebuildableFilter(), loader, nil))}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// No scheduler: only the construction-time sync can populate the filter.
			p := idp.newProvider(t, tc.opts()...)

			_, err := p.ValidateToken(t.Context(), revoked)
			require.ErrorIs(t, err, ErrTokenRevoked)
			_, err = p.ValidateToken(t.Context(), valid)
			require.NoError(t, err)
		})
	}
}

func TestNewProvider_InitialRevocationSyncFailure(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	idp.revokedStatus.Store(http.StatusInternalServerError)
	opts := func() []Option {
		return []Option{
			WithRevocationFilter(newRebuildableFilter()),
			WithRevocationLoader(&URLRevocationLoader{URL: idp.srv.URL + "/revoked"}),
		}
	}

	_, err := idp.tryNewProvider(t, opts()...)
	require.ErrorIs(t, err, ErrRevocationCheck)

	p, err := idp.tryNewProvider(t, append(opts(), WithRevocationFailOpen())...)
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestServiceConfig_AnyOfScopes(t *testing.T) {
	t.Parallel()

	cfg := &ValidationRulesConfig{Scopes: &ScopesValidationConfig{AnyOf: []string{"admin", "ops"}}}
	opts, err := cfg.ToValidationOptions()
	require.NoError(t, err)

	idp := newTestIdP(t)
	p := idp.newProvider(t, WithDefaultValidationOptions(append(opts, WithValidationAudience(testAudience))...))

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

			raw := idp.sign(t, idp.claims(map[string]any{"scope": tc.scope}))
			_, err := p.ValidateToken(t.Context(), raw)
			if tc.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrCELValidation)
		})
	}
}

func TestValidate_PerCallOptionsKeepDefaultCELRules(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	defaults := WithDefaultValidationOptions(
		WithValidationAudience(testAudience),
		WithValidationCelRules(CELValidationRule{Name: "admin-only", Expression: "claims.role == 'admin'"}),
	)
	user := idp.sign(t, idp.claims(map[string]any{"role": "user"}))
	admin := idp.sign(t, idp.claims(map[string]any{"role": "admin"}))

	for name, opts := range map[string][]Option{
		"no cache":   {defaults},
		"with cache": {defaults, WithTokenCache(newMemCacher())},
	} {
		p := idp.newProvider(t, opts...)
		for range 2 { // second round exercises cache hits
			_, err := p.ValidateToken(t.Context(), user)
			require.ErrorIs(t, err, ErrCELValidation, name)

			_, err = p.ValidateTokenWithOptions(t.Context(), user, WithValidationAudience(testAudience))
			require.ErrorIs(t, err, ErrCELValidation, "%s: an unrelated per-call option must not drop the default CEL rule", name)

			_, err = p.ValidateTokenWithOptions(t.Context(), admin, WithValidationAudience(testAudience))
			require.NoError(t, err, name)
		}
	}
}

func TestIntrospection_CacheBoundToAuthority(t *testing.T) {
	t.Parallel()

	idpA, idpB := newTestIdP(t), newTestIdP(t)
	idpB.active.Store(false)
	cache := newMemCacher()
	pA := idpA.newProvider(t, WithTokenCache(cache), WithIntrospection("client", "secret"))
	pB := idpB.newProvider(t, WithTokenCache(cache), WithIntrospection("client", "secret"))

	respA, err := pA.IntrospectToken(t.Context(), "opaque-token")
	require.NoError(t, err)
	require.True(t, respA.Active)

	respB, err := pB.IntrospectToken(t.Context(), "opaque-token")
	require.NoError(t, err)
	require.False(t, respB.Active, "B must not reuse A's cached introspection answer")
	require.EqualValues(t, 1, idpB.introspections.Load())
}

func TestValidate_CacheHitRequiresCurrentSigningKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		newKid string
		fresh  bool // validate through a newly constructed provider sharing the cache
	}{
		{name: "key removed by refresh", newKid: "k2"},
		{name: "key replaced under the same kid", newKid: "k1"},
		{name: "fresh provider sharing an older cache", newKid: "k2", fresh: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			cache := newMemCacher()
			p := idp.newProvider(t, WithTokenCache(cache))
			raw := idp.sign(t, idp.claims(nil))

			_, err := p.ValidateToken(t.Context(), raw)
			require.NoError(t, err)
			_, err = p.ValidateToken(t.Context(), raw)
			require.NoError(t, err, "precondition: cache hit while the key is trusted")

			idp.rotate(t, tc.newKid)
			if tc.fresh {
				p = idp.newProvider(t, WithTokenCache(cache))
			} else {
				require.NoError(t, p.RefreshJWKS(t.Context()))
			}

			_, err = p.ValidateToken(t.Context(), raw)
			require.ErrorIs(t, err, ErrTokenInvalid, "a cached verification must not outlive its signing key")
		})
	}
}

func TestRefreshJWKS_NoopAfterClose(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	p := idp.newProvider(t)
	refresh := p.RegisterJWKSRefreshSchedulerFunc() // e.g. registered with an external scheduler
	require.NoError(t, refresh(t.Context()))

	p.Close()
	require.ErrorIs(t, refresh(t.Context()), context.Canceled)
}

// A key rotated between the signature check and the cache write must not be
// recorded as the key that verified the token.
func TestValidate_CacheRecordsKeyUsedForVerification(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	p := idp.newProvider(t, WithTokenCache(newMemCacher()))
	raw := idp.sign(t, idp.claims(nil))

	// Same kid, new key material: replace the JWKS right after the
	// verification resolved the old key, before the entry is cached.
	inner := p.keyResolver
	var rotated atomic.Bool
	p.keyResolver = authjwt.KeyResolverFunc(func(ctx context.Context, hdr authjwt.Header, c authjwt.Claims) (authjwt.VerificationKey, error) {
		vk, err := inner.ResolveKey(ctx, hdr, c)
		if err == nil && rotated.CompareAndSwap(false, true) {
			idp.rotate(t, "k1")
			require.NoError(t, p.RefreshJWKS(ctx))
		}
		return vk, err
	})

	_, err := p.ValidateToken(t.Context(), raw)
	require.NoError(t, err, "the first validation verified against the then-current key")
	require.True(t, rotated.Load())

	_, err = p.ValidateToken(t.Context(), raw)
	require.ErrorIs(t, err, ErrTokenInvalid, "the old token must be re-verified against the rotated key")
}

func TestServiceConfig_AnyOfScopesAndCustomCELRulesBothEnforced(t *testing.T) {
	t.Parallel()

	cfg := &ValidationRulesConfig{
		Scopes:   &ScopesValidationConfig{AnyOf: []string{"admin"}},
		CELRules: []CELRuleDefinition{{Name: "users-only", Expression: "claims.role == 'user'"}},
	}
	opts, err := cfg.ToValidationOptions()
	require.NoError(t, err)

	idp := newTestIdP(t)
	p := idp.newProvider(t, WithTokenCache(newMemCacher()),
		WithDefaultValidationOptions(append(opts, WithValidationAudience(testAudience))...))

	noScope := idp.sign(t, idp.claims(map[string]any{"role": "user", "scope": "read"}))
	wrongRole := idp.sign(t, idp.claims(map[string]any{"role": "guest", "scope": "admin"}))
	both := idp.sign(t, idp.claims(map[string]any{"role": "user", "scope": "admin"}))

	for range 2 { // second round exercises cache hits
		_, err = p.ValidateToken(t.Context(), noScope)
		require.ErrorIs(t, err, ErrCELValidation, "any_of must survive custom CEL rules")
		_, err = p.ValidateToken(t.Context(), wrongRole)
		require.ErrorIs(t, err, ErrCELValidation)
		_, err = p.ValidateToken(t.Context(), both)
		require.NoError(t, err)
	}
}
