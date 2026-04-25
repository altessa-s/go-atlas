// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type mockLimiter struct {
	err error
}

func (m *mockLimiter) Allow(_ context.Context, _ string) error {
	return m.err
}

func TestNewRoundTripper(t *testing.T) {
	rt := NewRoundTripper(http.DefaultTransport, &mockLimiter{})
	require.NotNil(t, rt)
}

func TestNewRoundTripper_NilNext(t *testing.T) {
	rt := NewRoundTripper(nil, &mockLimiter{})
	require.NotNil(t, rt)
	// next should default to http.DefaultTransport
	require.NotNil(t, rt.next)
}

func TestRoundTripper_RoundTrip_Allowed(t *testing.T) {
	next := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	rt := NewRoundTripper(next, &mockLimiter{})

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRoundTripper_RoundTrip_Denied(t *testing.T) {
	rt := NewRoundTripper(http.DefaultTransport, &mockLimiter{err: errors.New("rate limited")})

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	_, err := rt.RoundTrip(req)
	require.Error(t, err)
}

func TestRoundTripper_RoundTrip_HostnameExtracted(t *testing.T) {
	var capturedKey string
	limiter := &capturingLimiter{capturedKey: &capturedKey}
	next := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	rt := NewRoundTripper(next, limiter)

	req, _ := http.NewRequest("GET", "http://api.example.com:8080/test", nil)
	_, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, "api.example.com", capturedKey)
}

type capturingLimiter struct {
	capturedKey *string
}

func (l *capturingLimiter) Allow(_ context.Context, key string) error {
	*l.capturedKey = key
	return nil
}
