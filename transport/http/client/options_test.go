// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	require.Equal(t, DefaultRetryMax, opts.retryMax)
	require.Equal(t, DefaultRetryWaitMin, opts.retryWaitMin)
	require.Equal(t, DefaultRetryWaitMax, opts.retryWaitMax)
	require.Equal(t, uint32(DefaultBreakerMaxRequests), opts.breakerMaxRequests)
	require.Equal(t, DefaultBreakerInterval, opts.breakerInterval)
	require.Equal(t, DefaultBreakerTimeout, opts.breakerTimeout)
	require.NotNil(t, opts.client)
}

func TestWithRetryMax(t *testing.T) {
	opts := newOptions(WithRetryMax(10))
	require.Equal(t, 10, opts.retryMax)
}

func TestWithRetryWait(t *testing.T) {
	opts := newOptions(WithRetryWait(1*time.Second, 30*time.Second))
	require.Equal(t, 1*time.Second, opts.retryWaitMin)
	require.Equal(t, 30*time.Second, opts.retryWaitMax)
}

func TestWithBreakerName(t *testing.T) {
	opts := newOptions(WithBreakerName("my-breaker"))
	require.Equal(t, "my-breaker", opts.breakerName)
}

func TestWithBreakerName_StringPtr(t *testing.T) {
	name := "ptr-breaker"
	opts := newOptions(WithBreakerName(&name))
	require.Equal(t, "ptr-breaker", opts.breakerName)
}

func TestWithBreakerName_NilPtr(t *testing.T) {
	opts := newOptions(WithBreakerName[*string](nil))
	require.Equal(t, "", opts.breakerName)
}

func TestWithClient(t *testing.T) {
	c := &http.Client{}
	opts := newOptions(WithClient(c))
	require.Equal(t, c, opts.client)
}

func TestWithClient_Nil(t *testing.T) {
	opts := newOptions(WithClient(nil))
	require.NotNil(t, opts.client)
}

func TestWithLogger(t *testing.T) {
	l := slog.New(slog.DiscardHandler)
	opts := newOptions(WithLogger(l))
	require.Equal(t, l, opts.logger)
}

func TestWithLogger_Nil(t *testing.T) {
	opts := newOptions(WithLogger(nil))
	require.NotNil(t, opts.logger)
}

func TestWithMaxResponseSize(t *testing.T) {
	opts := newOptions(WithMaxResponseSize(1024))
	require.Equal(t, int64(1024), opts.maxResponseSize)
}

func TestWithBreakerInterval_Negative(t *testing.T) {
	opts := newOptions(WithBreakerInterval(-1))
	require.Equal(t, DefaultBreakerInterval, opts.breakerInterval)
}

func TestWithBreakerTimeout_Negative(t *testing.T) {
	opts := newOptions(WithBreakerTimeout(-1))
	require.Equal(t, DefaultBreakerTimeout, opts.breakerTimeout)
}

func TestWithCircuitBreakerSettings(t *testing.T) {
	settings := &CircuitBreakerSettings{MaxRequests: 5, Timeout: 10 * time.Second}
	opts := newOptions(WithCircuitBreakerSettings("api.test", settings))
	require.NotNil(t, opts.hostBreakerSettings)
	require.Equal(t, settings, opts.hostBreakerSettings["api.test"])
}

func TestWithTransport_Nil(t *testing.T) {
	opts := newOptions(WithTransport(nil))
	require.Nil(t, opts.transport)
}

func TestWithMetricsSubsystem(t *testing.T) {
	opts := newOptions(WithMetricsSubsystem("egrul"))
	require.Equal(t, "egrul", opts.metricsSubsystem)
}

func TestWithMetricsSubsystem_StringPtr(t *testing.T) {
	sub := "kfocus"
	opts := newOptions(WithMetricsSubsystem(&sub))
	require.Equal(t, "kfocus", opts.metricsSubsystem)
}

func TestWithMetricsSubsystem_NilPtr(t *testing.T) {
	opts := newOptions(WithMetricsSubsystem[*string](nil))
	require.Equal(t, DefaultMetricsSubsystem, opts.metricsSubsystem)
}

func TestWithMetricsSubsystem_Empty(t *testing.T) {
	opts := newOptions(WithMetricsSubsystem(""))
	require.Equal(t, DefaultMetricsSubsystem, opts.metricsSubsystem)
}

func TestWithMetricsSubsystem_Default(t *testing.T) {
	opts := newOptions()
	require.Equal(t, DefaultMetricsSubsystem, opts.metricsSubsystem)
}

func TestDefaultConstants(t *testing.T) {
	require.Greater(t, DefaultRetryWaitMin, time.Duration(0))
	require.Greater(t, DefaultRetryWaitMax, time.Duration(0))
	require.Greater(t, DefaultRetryMax, 0)
	require.Greater(t, DefaultBreakerTimeout, time.Duration(0))
	require.Greater(t, DefaultBreakerInterval, time.Duration(0))
	require.NotEqual(t, 0, DefaultBreakerMaxRequests)
}
