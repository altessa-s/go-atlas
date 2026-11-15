// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func fieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func on() httpconfig.BaseMiddleware {
	return httpconfig.BaseMiddleware{Enabled: true}
}

func TestDefault(t *testing.T) {
	t.Parallel()

	c := httpconfig.Default()
	require.Equal(t, "0.0.0.0:9080", c.ListenAddress)
	require.Equal(t, 10*time.Second, c.ReadHeaderTimeout)
	require.NoError(t, c.Validate())

	mws := httpconfig.DefaultMiddlewares()
	pprof := httpconfig.DefaultPprof()
	require.False(t, pprof.Enabled)
	c.Middlewares = &mws
	c.Pprof = &pprof
	c.TLS = &httpconfig.TLS{
		Server:          &tlsconfig.Server{Provider: tlsconfig.ProviderTypeFile, MinTLSVersion: "1.2"},
		StrictTransport: &httpconfig.STS{MaxAge: 24 * time.Hour},
	}
	require.NoError(t, c.Validate())
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	c := httpconfig.Default()
	c.ListenAddress = "bogus"
	c.Middlewares = &httpconfig.Middlewares{RequestId: &httpconfig.RequestIDMiddleware{BaseMiddleware: on()}}
	c.TLS = &httpconfig.TLS{}
	verrs := fieldErrors(t, c.Validate())
	for _, f := range []string{"ListenAddress", "Middlewares", "TLS"} {
		require.Contains(t, verrs, f)
	}
}

func TestTLS_Validate(t *testing.T) {
	t.Parallel()

	require.Contains(t, fieldErrors(t, (&httpconfig.TLS{}).Validate()), "Server")

	verrs := fieldErrors(t, (&httpconfig.TLS{
		Server:          &tlsconfig.Server{Provider: "acme", MinTLSVersion: "1.2"},
		StrictTransport: &httpconfig.STS{MaxAge: time.Minute},
	}).Validate())
	require.Contains(t, verrs, "Provider", "embedded server settings are validated")
	require.Contains(t, verrs, "StrictTransport")

	require.NoError(t, (&httpconfig.STS{MaxAge: 8760 * time.Hour}).Validate())
	require.Contains(t, fieldErrors(t, (&httpconfig.STS{MaxAge: time.Minute}).Validate()), "MaxAge")
}

func TestMiddlewares_DefaultsDisabled(t *testing.T) {
	t.Parallel()

	type mw interface {
		IsEnabled() bool
		Validate() error
	}

	d := httpconfig.DefaultMiddlewares()
	require.NoError(t, d.Validate())
	for name, m := range map[string]mw{
		"bodyLimit": d.BodyLimit, "cors": d.Cors, "idempotency": d.Idempotency, "ipACL": d.IpAcl,
		"geoACL": d.GeoAcl, "limiter": d.Limiter, "logger": d.Logger, "metrics": d.Metrics,
		"realIP": d.RealIp, "recovery": d.Recovery, "requestID": d.RequestId,
		"securityHeaders": d.SecurityHeaders, "tracing": d.Tracing,
	} {
		require.False(t, m.IsEnabled(), name)
		require.NoError(t, m.Validate(), name)
	}

	var nilCors *httpconfig.CORSMiddleware
	require.False(t, nilCors.IsEnabled())

	enabledOK := []mw{
		&httpconfig.MetricsMiddleware{BaseMiddleware: on()},
		&httpconfig.RecoveryMiddleware{BaseMiddleware: on()},
		&httpconfig.SecurityHeadersMiddleware{BaseMiddleware: on()},
	}
	for _, m := range enabledOK {
		require.True(t, m.IsEnabled())
		require.NoError(t, m.Validate())
	}

	bad := httpconfig.RecoveryMiddleware{BaseMiddleware: on()}
	bad.IgnorePaths = []string{""}
	bad.IgnorePatterns = []string{"("}
	verrs := fieldErrors(t, bad.Validate())
	require.Contains(t, verrs, "IgnorePaths")
	require.Contains(t, verrs, "IgnorePatterns")
}

func TestBodyLimitMiddleware_Validate(t *testing.T) {
	t.Parallel()

	b := httpconfig.DefaultBodyLimitMiddleware()
	b.Enabled = true
	require.True(t, b.IsEnabled())
	require.NoError(t, b.Validate())

	b.MaxSize = -1
	require.Contains(t, fieldErrors(t, b.Validate()), "MaxSize")
}

func TestCORSMiddleware_Validate(t *testing.T) {
	t.Parallel()

	c := httpconfig.DefaultCORSMiddleware()
	c.Enabled = true
	c.AllowedOrigins = []string{"https://app.example.com"}
	require.True(t, c.IsEnabled())
	require.NoError(t, c.Validate())

	c.AllowedOrigins = []string{""}
	c.MaxAge = -1
	c.OptionsSuccessStatus = 700
	verrs := fieldErrors(t, c.Validate())
	for _, f := range []string{"AllowedOrigins", "MaxAge", "OptionsSuccessStatus"} {
		require.Contains(t, verrs, f)
	}
}

func TestACLMiddlewares_Validate(t *testing.T) {
	t.Parallel()

	ip := httpconfig.DefaultIPACLMiddleware()
	ip.Enabled = true
	ip.Rules = []middlewareconfig.IPACLRule{{Denylist: []string{"192.0.2.0/24"}}}
	require.True(t, ip.IsEnabled())
	require.NoError(t, ip.Validate())

	ip.DefaultPolicy = "maybe"
	ip.FallbackBehavior = ""
	ip.Rules = []middlewareconfig.IPACLRule{{Denylist: []string{"nope"}}}
	verrs := fieldErrors(t, ip.Validate())
	for _, f := range []string{"DefaultPolicy", "FallbackBehavior", "Rules"} {
		require.Contains(t, verrs, f)
	}

	geo := httpconfig.DefaultGeoACLMiddleware()
	geo.Enabled = true
	geo.Rules = []middlewareconfig.GeoACLRule{{AllowContinents: []string{"EU"}}}
	require.NoError(t, geo.Validate())

	geo.Rules = []middlewareconfig.GeoACLRule{{AllowContinents: []string{"XX"}}}
	require.Contains(t, fieldErrors(t, geo.Validate()), "Rules")
}

func TestLimiterMiddleware_Validate(t *testing.T) {
	t.Parallel()

	l := httpconfig.DefaultLimiterMiddleware()
	l.Enabled = true
	require.True(t, l.IsEnabled())
	require.NoError(t, l.Validate())

	l.FallbackBehavior = "ignore"
	require.Contains(t, fieldErrors(t, l.Validate()), "FallbackBehavior")
}

func TestRealIPMiddleware_Validate(t *testing.T) {
	t.Parallel()

	r := httpconfig.DefaultRealIPMiddleware()
	r.Enabled = true
	require.True(t, r.IsEnabled())
	require.NoError(t, r.Validate())

	r.Headers = []string{""}
	r.TrustedProxies = []string{""}
	verrs := fieldErrors(t, r.Validate())
	require.Contains(t, verrs, "Headers")
	require.Contains(t, verrs, "TrustedProxies")
}

func TestRequestIDMiddleware_Validate(t *testing.T) {
	t.Parallel()

	r := httpconfig.DefaultRequestIDMiddleware()
	r.Enabled = true
	require.True(t, r.IsEnabled())
	require.NoError(t, r.Validate())

	r.HeaderName = ""
	require.Contains(t, fieldErrors(t, r.Validate()), "HeaderName")
}

func TestTracingMiddleware_Validate(t *testing.T) {
	t.Parallel()

	tr := httpconfig.DefaultTracingMiddleware()
	tr.Enabled = true
	require.True(t, tr.IsEnabled())
	require.Contains(t, tr.RedactedHeaders, "Authorization")
	require.NoError(t, tr.Validate())

	tr.RedactedHeaders = []string{""}
	require.Contains(t, fieldErrors(t, tr.Validate()), "RedactedHeaders")
}

func TestLoggerAndIdempotency_IsEnabled(t *testing.T) {
	t.Parallel()

	l := httpconfig.DefaultLoggerMiddleware()
	require.False(t, l.IsEnabled())
	l.Enabled = true
	require.True(t, l.IsEnabled())

	i := httpconfig.DefaultIdempotencyMiddleware()
	require.False(t, i.IsEnabled())
	i.Enabled = true
	require.True(t, i.IsEnabled())
}
