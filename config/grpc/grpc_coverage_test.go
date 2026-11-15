// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
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

func enabled() grpcconfig.BaseInterceptor {
	return grpcconfig.BaseInterceptor{Enabled: true}
}

func TestDefault(t *testing.T) {
	t.Parallel()

	c := grpcconfig.Default()
	require.Equal(t, "0.0.0.0:7777", c.ListenAddress)
	require.False(t, c.Reflection)
	require.NoError(t, c.Validate())

	ics := grpcconfig.DefaultInterceptors()
	c.Interceptors = &ics
	c.KeepAlive = &grpcconfig.KeepAlive{Time: 30 * time.Second, EnforcementPolicy: &grpcconfig.KeepAliveEnforcementPolicy{MinTime: time.Second}}
	c.ConnectionTimeout = new(5 * time.Second)
	c.MaxSendMsgSize = new(1024)
	c.MaxRecvMsgSize = new(grpcconfig.MaxMessageSize)
	require.NoError(t, c.Validate())
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	t.Run("listen address", func(t *testing.T) {
		t.Parallel()
		c := grpcconfig.Default()
		c.ListenAddress = "nowhere"
		require.Contains(t, fieldErrors(t, c.Validate()), "ListenAddress")
	})

	t.Run("message size limits", func(t *testing.T) {
		t.Parallel()
		c := grpcconfig.Default()
		c.MaxSendMsgSize = new(0)
		require.ErrorIs(t, c.Validate(), coreerrs.ErrInvalidArgument)

		c = grpcconfig.Default()
		c.MaxRecvMsgSize = new(grpcconfig.MaxMessageSize + 1)
		require.ErrorIs(t, c.Validate(), coreerrs.ErrInvalidArgument)
	})

	t.Run("nested sections", func(t *testing.T) {
		t.Parallel()
		c := grpcconfig.Default()
		c.Interceptors = &grpcconfig.Interceptors{
			RequestId: &grpcconfig.RequestIDInterceptor{BaseInterceptor: enabled()},
		}
		c.TLS = &grpcconfig.TLS{
			Server: validTLSServer(),
			Client: &tlsconfig.ClientAuth{CACerts: []string{"/does/not/exist.pem"}},
		}
		verrs := fieldErrors(t, c.Validate())
		require.Contains(t, verrs, "Interceptors")
		require.Contains(t, verrs, "TLS")
	})
}

func validTLSServer() *tlsconfig.Server {
	return &tlsconfig.Server{Provider: tlsconfig.ProviderTypeFile, MinTLSVersion: "1.2"}
}

func TestTLS_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&grpcconfig.TLS{Server: validTLSServer()}).Validate())
	require.NoError(t, (&grpcconfig.TLS{
		Server: validTLSServer(), Client: &tlsconfig.ClientAuth{AllowCertAuth: true},
	}).Validate())

	// The embedded server block is required and validated, as for HTTP. Its
	// errors are inlined, matching its inline YAML keys.
	require.Contains(t, fieldErrors(t, (&grpcconfig.TLS{}).Validate()), "Server")
	verrs := fieldErrors(t, (&grpcconfig.TLS{
		Server: &tlsconfig.Server{Provider: "ftp", MinTLSVersion: "1.0"},
	}).Validate())
	require.Contains(t, verrs, "Provider")
	require.Contains(t, verrs, "MinTLSVersion")
}

func TestKeepAlive_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&grpcconfig.KeepAlive{}).Validate(), "zero durations keep the gRPC defaults")
	require.NoError(t, (&grpcconfig.KeepAlive{Time: time.Minute, Timeout: time.Second}).Validate())

	err := (&grpcconfig.KeepAlive{MaxConnectionIdle: -time.Second, Timeout: -time.Second}).Validate()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	require.Contains(t, verrs, "MaxConnectionIdle")
	require.Contains(t, verrs, "Timeout")
}

func TestBaseInterceptors(t *testing.T) {
	t.Parallel()

	type validatable interface {
		IsEnabled() bool
		Validate() error
	}

	defaults := grpcconfig.DefaultInterceptors()
	require.NoError(t, defaults.Validate())
	for name, ic := range map[string]validatable{
		"auth":      defaults.Auth,
		"cache":     defaults.Cache,
		"errStatus": defaults.ErrStatus,
		"fieldMask": defaults.FieldMask,
		"geoACL":    defaults.GeoAcl,
		"health":    defaults.Health,
		"idem":      defaults.Idempotency,
		"ipACL":     defaults.IpAcl,
		"limiter":   defaults.Limiter,
		"logger":    defaults.Logger,
		"metrics":   defaults.Metrics,
		"realIP":    defaults.RealIp,
		"recovery":  defaults.Recovery,
		"requestID": defaults.RequestId,
		"tracing":   defaults.Tracing,
	} {
		require.False(t, ic.IsEnabled(), name)
		require.NoError(t, ic.Validate(), name)
	}

	var nilAuth *grpcconfig.AuthInterceptor
	require.False(t, nilAuth.IsEnabled())

	// Enabled interceptors with only the base filter must validate cleanly,
	// and an empty ignore-method entry must be rejected for each of them.
	enabledOnly := []validatable{
		&grpcconfig.AuthInterceptor{BaseInterceptor: enabled()},
		&grpcconfig.ErrStatusInterceptor{BaseInterceptor: enabled()},
		&grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled()},
		&grpcconfig.HealthInterceptor{BaseInterceptor: enabled()},
		&grpcconfig.RecoveryInterceptor{BaseInterceptor: enabled()},
		&grpcconfig.TracingInterceptor{BaseInterceptor: enabled()},
	}
	for _, ic := range enabledOnly {
		require.True(t, ic.IsEnabled())
		require.NoError(t, ic.Validate())
	}

	bad := grpcconfig.HealthInterceptor{BaseInterceptor: enabled()}
	bad.IgnoreMethods = []string{""}
	require.Contains(t, fieldErrors(t, bad.Validate()), "IgnoreMethods")
}

func TestCacheInterceptor_Validate(t *testing.T) {
	t.Parallel()

	c := grpcconfig.DefaultCacheInterceptor()
	c.Enabled = true
	require.True(t, c.IsEnabled())
	require.NoError(t, c.Validate())

	c.SuccessTTL = time.Second
	c.ErrorTTL = 0
	c.CompressionPreset = "ultra"
	verrs := fieldErrors(t, c.Validate())
	for _, f := range []string{"SuccessTTL", "ErrorTTL", "CompressionPreset"} {
		require.Contains(t, verrs, f)
	}
}

func TestACLInterceptors_Validate(t *testing.T) {
	t.Parallel()

	geo := grpcconfig.DefaultGeoACLInterceptor()
	geo.Enabled = true
	geo.Rules = []middlewareconfig.GeoACLRule{{AllowCountries: []string{"DE"}}}
	require.True(t, geo.IsEnabled())
	require.NoError(t, geo.Validate())

	geo.DefaultPolicy = "maybe"
	geo.FallbackBehavior = "ignore"
	geo.Rules = []middlewareconfig.GeoACLRule{{AllowCountries: []string{"germany"}}}
	verrs := fieldErrors(t, geo.Validate())
	for _, f := range []string{"DefaultPolicy", "FallbackBehavior", "Rules"} {
		require.Contains(t, verrs, f)
	}

	ip := grpcconfig.DefaultIPACLInterceptor()
	ip.Enabled = true
	ip.Rules = []middlewareconfig.IPACLRule{{Allowlist: []string{"10.0.0.0/8"}}}
	require.True(t, ip.IsEnabled())
	require.NoError(t, ip.Validate())

	ip.DefaultPolicy = ""
	ip.Rules = []middlewareconfig.IPACLRule{{Allowlist: []string{"nope"}}}
	verrs = fieldErrors(t, ip.Validate())
	require.Contains(t, verrs, "DefaultPolicy")
	require.Contains(t, verrs, "Rules")
}

func TestLimiterInterceptor_Validate(t *testing.T) {
	t.Parallel()

	l := grpcconfig.DefaultLimiterInterceptor()
	l.Enabled = true
	require.True(t, l.IsEnabled())
	require.NoError(t, l.Validate())

	l.FallbackBehavior = "maybe"
	require.Contains(t, fieldErrors(t, l.Validate()), "FallbackBehavior")
}

func TestLoggerInterceptor_Validate(t *testing.T) {
	t.Parallel()

	l := grpcconfig.DefaultLoggerInterceptor()
	l.Enabled = true
	require.True(t, l.IsEnabled())
	require.NoError(t, l.Validate())

	l.TimeFormat = "kitchen"
	require.Contains(t, fieldErrors(t, l.Validate()), "TimeFormat")
}

func TestMetricsInterceptor_Validate(t *testing.T) {
	t.Parallel()

	m := grpcconfig.DefaultMetricsInterceptor()
	m.Enabled = true
	m.DurationBuckets = []float64{0.1, 0.5, 1}
	require.True(t, m.IsEnabled())
	require.NoError(t, m.Validate())

	m.DurationBuckets = []float64{1, 0.5}
	m.SizeBuckets = []float64{10, 10}
	m.StreamSamplingRate = 1.5
	m.StreamSamplingStrategy = "random"
	verrs := fieldErrors(t, m.Validate())
	for _, f := range []string{"DurationBuckets", "SizeBuckets", "StreamSamplingRate", "StreamSamplingStrategy"} {
		require.Contains(t, verrs, f)
	}
}

func TestRealIPInterceptor_Validate(t *testing.T) {
	t.Parallel()

	r := grpcconfig.DefaultRealIPInterceptor()
	r.Enabled = true
	r.TrustedProxies = []string{"10.0.0.0/8"}
	r.Headers = []string{"x-forwarded-for"}
	require.True(t, r.IsEnabled())
	require.NoError(t, r.Validate())

	r.TrustedProxies = []string{"bad"}
	r.TrustedPeers = []string{"bad"}
	r.Headers = []string{""}
	r.CacheSize = -1
	verrs := fieldErrors(t, r.Validate())
	for _, f := range []string{"TrustedProxies", "TrustedPeers", "Headers", "CacheSize"} {
		require.Contains(t, verrs, f)
	}
}

func TestRequestIDInterceptor_Validate(t *testing.T) {
	t.Parallel()

	r := grpcconfig.DefaultRequestIDInterceptor()
	r.Enabled = true
	require.True(t, r.IsEnabled())
	require.NoError(t, r.Validate())

	r.HeaderName = ""
	require.Contains(t, fieldErrors(t, r.Validate()), "HeaderName")
}

type filterHolder struct {
	grpcconfig.InterceptorFilter
}

func (h *filterHolder) Validate() error {
	return validation.ValidateStruct(h, h.FilterValidationRules(&h.InterceptorFilter)...)
}

func TestInterceptorFilter_FilterValidationRules(t *testing.T) {
	t.Parallel()

	ok := filterHolder{grpcconfig.InterceptorFilter{IgnoreMethods: []string{"/svc/M"}, IgnorePatterns: []string{"^/svc/"}}}
	require.NoError(t, ok.Validate())

	bad := filterHolder{grpcconfig.InterceptorFilter{IgnoreMethods: []string{""}, IgnorePatterns: []string{"("}}}
	verrs := fieldErrors(t, bad.Validate())
	require.Contains(t, verrs, "IgnoreMethods")
	require.Contains(t, verrs, "IgnorePatterns")
}
