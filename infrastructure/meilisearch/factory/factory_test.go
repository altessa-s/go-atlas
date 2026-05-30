// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"

	mfactory "github.com/altessa-s/go-atlas/infrastructure/meilisearch/factory"
)

// TestNew_AcceptsNilConfig keeps the construction-stage contract: a nil
// config is allowed at New time; the failure is deferred to Build so
// the fluent chain can complete even when the YAML loader omitted the
// Meilisearch block.
func TestNew_AcceptsNilConfig(t *testing.T) {
	t.Parallel()

	b := mfactory.New(nil)
	require.NotNil(t, b, "New must always return a builder, even with nil config")
}

// TestBuild_NilConfigReturnsErrConfigRequired pins the PR-43 review
// fix that exported ErrConfigRequired so dynamic config pipelines can
// branch on it. Before the fix Build returned a fmt.Errorf string,
// unreachable via errors.Is.
func TestBuild_NilConfigReturnsErrConfigRequired(t *testing.T) {
	t.Parallel()

	_, err := mfactory.New(nil).Build(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, mfactory.ErrConfigRequired),
		"Build must return a sentinel-compatible ErrConfigRequired so YAML-loader pipelines can branch on it")
}

// TestBuild_HappyPath_HealthProbeSucceeds drives the factory end-to-end
// against an httptest server pretending to be Meilisearch. Confirms
// the synchronous startup health probe reaches the right URL with the
// right Authorization header.
func TestBuild_HappyPath_HealthProbeSucceeds(t *testing.T) {
	t.Parallel()

	var (
		gotPath string
		gotAuth string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"available"}`))
	}))
	defer srv.Close()

	client, err := mfactory.New(&config.Meilisearch{
		Host:   srv.URL,
		APIKey: config.Secret("secret-key"),
	}).
		UseLogger(slog.New(slog.DiscardHandler)).
		Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close() //nolint:errcheck // test cleanup

	require.Equal(t, "/health", gotPath, "factory must wire the SDK to call /health on startup")
	require.Equal(t, "Bearer secret-key", gotAuth,
		"APIKey from config must be threaded into the Authorization header via WithAPIKey")
}

// TestBuild_RegistersHealthChecker_WhenCoordinatorInjected pins the
// health.Coordinator integration. The check name defaults to
// "meilisearch" unless overridden via UseHealthServiceName.
func TestBuild_RegistersHealthChecker_WhenCoordinatorInjected(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"available"}`))
	}))
	defer srv.Close()

	coord := health.New()
	defer coord.Close()

	_, err := mfactory.New(&config.Meilisearch{Host: srv.URL}).
		UseHealthCoordinator(coord).
		Build(t.Context())
	require.NoError(t, err)

	services := collectServices(coord)
	require.Contains(t, services, "meilisearch",
		"injecting a health.Coordinator must register the default 'meilisearch' service")
}

// TestBuild_HealthServiceNameOverride confirms UseHealthServiceName
// changes the registered name. Useful when two Meilisearch instances
// share a single coordinator and would otherwise collide on the
// "meilisearch" default.
func TestBuild_HealthServiceNameOverride(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"available"}`))
	}))
	defer srv.Close()

	coord := health.New()
	defer coord.Close()

	_, err := mfactory.New(&config.Meilisearch{Host: srv.URL}).
		UseHealthCoordinator(coord).
		UseHealthServiceName("search-primary").
		Build(t.Context())
	require.NoError(t, err)

	services := collectServices(coord)
	require.Contains(t, services, "search-primary")
	require.NotContains(t, services, "meilisearch", "default name must NOT register when an override is set")
}

// TestBuild_TLSConfigThreadsThroughToHTTPClient is the regression
// guard for the TLS support added in this followup. Asserts the
// factory threads the TlsClient through to an http.Client with a
// real *tls.Config — proven by an httptest TLS server requiring the
// matching CA to succeed.
//
// SkipVerify=true is acceptable inside a test that controls both
// endpoints — Normalize() zeros it unless ATLAS_ALLOW_INSECURE_TLS is
// "true", so we set the env var via t.Setenv to mimic an explicit
// operator opt-in.
func TestBuild_TLSConfigThreadsThroughToHTTPClient(t *testing.T) {
	t.Setenv(config.EnvAllowInsecureTLS, "true")

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"available"}`))
	}))
	defer srv.Close()

	tlsCfg := &config.TlsClient{
		ServerName:     strings.TrimPrefix(srv.URL, "https://"),
		SkipVerify:     true,
		SkipVerifyMode: config.TLSSkipVerifyModeDisabled,
	}
	tlsCfg.Normalize()

	client, err := mfactory.New(&config.Meilisearch{
		Host: srv.URL,
		TLS:  tlsCfg,
	}).Build(t.Context())
	require.NoError(t, err, "TLS http.Client must be threaded through to the SDK so the health probe succeeds over HTTPS")
	require.NotNil(t, client)
	defer client.Close() //nolint:errcheck // test cleanup
}

// collectServices drains health.Coordinator.ListServices into a slice
// the test can assert against — the iterator API is one-shot.
func collectServices(coord *health.Coordinator) []string {
	var out []string
	for s := range coord.ListServices() {
		out = append(out, s)
	}
	return out
}
