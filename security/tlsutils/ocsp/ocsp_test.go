// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

func TestNewOCSPStapler(t *testing.T) {
	s := NewOCSPStapler()
	require.NotNil(t, s)
	require.NotNil(t, s.cache)
	require.NotNil(t, s.httpClient)
	require.NotNil(t, s.logger)
}

func TestNewOCSPStapler_WithCompression(t *testing.T) {
	s := NewOCSPStapler(WithCompression())
	require.True(t, s.enableCompression)

	s2 := NewOCSPStapler()
	require.False(t, s2.enableCompression)
}

func TestGetOCSPStaple_NilCert(t *testing.T) {
	s := NewOCSPStapler()
	_, err := s.GetOCSPStaple(t.Context(), nil)
	require.Error(t, err)
}

func TestGetOCSPStaple_EmptyCert(t *testing.T) {
	s := NewOCSPStapler()
	cert := &tls.Certificate{}
	_, err := s.GetOCSPStaple(t.Context(), cert)
	require.Error(t, err)
}

func TestNeedsRefresh_NilCert(t *testing.T) {
	s := NewOCSPStapler()
	require.False(t, s.NeedsRefresh(nil))
}

func TestNeedsRefresh_EmptyCert(t *testing.T) {
	s := NewOCSPStapler()
	require.False(t, s.NeedsRefresh(&tls.Certificate{}))
}

func TestRunRefreshCycle_NilCert(t *testing.T) {
	s := NewOCSPStapler()
	err := s.RunRefreshCycle(t.Context(), nil)
	require.Error(t, err)
}

func TestRunRefreshCycle_EmptyCert(t *testing.T) {
	s := NewOCSPStapler()
	err := s.RunRefreshCycle(t.Context(), &tls.Certificate{})
	require.Error(t, err)
}

func TestStapleOCSPToConfig_NilConfig(t *testing.T) {
	s := NewOCSPStapler()
	err := StapleOCSPToConfig(nil, s)
	require.Error(t, err)
}

func TestStapleOCSPToConfig_NilStapler(t *testing.T) {
	config := &tls.Config{}
	err := StapleOCSPToConfig(config, nil)
	require.Error(t, err)
}

func TestRemoveExpiredEntries(t *testing.T) {
	s := NewOCSPStapler()

	// Add an expired entry
	s.cache["expired"] = &ocspCacheEntry{
		response:   []byte("expired"),
		nextUpdate: time.Now().Add(-time.Hour),
	}

	// Add a valid entry
	s.cache["valid"] = &ocspCacheEntry{
		response:   []byte("valid"),
		nextUpdate: time.Now().Add(time.Hour),
	}

	s.removeExpiredEntries()

	_, hasExpired := s.cache["expired"]
	require.False(t, hasExpired, "expired entry should have been removed")
	_, hasValid := s.cache["valid"]
	require.True(t, hasValid, "valid entry should still exist")
}

func TestRunRefreshAll_NotSchedulerManaged(t *testing.T) {
	s := NewOCSPStapler()
	// No items in cache, should succeed
	require.NoError(t, s.RunRefreshAll(t.Context()))
}

func TestRunRefreshAll_SchedulerManaged(t *testing.T) {
	s := NewOCSPStapler()
	_ = s.RegisterRefreshAllSchedulerFunc()

	err := s.RunRefreshAll(t.Context())
	require.ErrorIs(t, err, corescheduler.ErrSchedulerManaged)
}

func TestRegisterRefreshAllSchedulerFunc(t *testing.T) {
	s := NewOCSPStapler()
	fn := s.RegisterRefreshAllSchedulerFunc()
	require.NotNil(t, fn)
	require.True(t, s.refreshAllTask.Registered())
}
