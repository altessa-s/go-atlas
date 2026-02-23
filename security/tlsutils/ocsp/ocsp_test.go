// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"bytes"
	"crypto/tls"
	"errors"
	"testing"
	"time"
)

func TestNewOCSPStapler(t *testing.T) {
	s := NewOCSPStapler()
	if s == nil {
		t.Fatal("NewOCSPStapler() returned nil")
	}
	if s.cache == nil {
		t.Error("cache is nil")
	}
	if s.httpClient == nil {
		t.Error("httpClient is nil")
	}
	if s.logger == nil {
		t.Error("logger is nil")
	}
}

func TestNewOCSPStapler_WithCompression(t *testing.T) {
	s := NewOCSPStapler(WithCompression())
	if !s.enableCompression {
		t.Error("compression should be enabled")
	}

	s2 := NewOCSPStapler()
	if s2.enableCompression {
		t.Error("compression should be disabled by default")
	}
}

func TestGetOCSPStaple_NilCert(t *testing.T) {
	s := NewOCSPStapler()
	_, err := s.GetOCSPStaple(t.Context(), nil)
	if err == nil {
		t.Error("GetOCSPStaple(nil) should return error")
	}
}

func TestGetOCSPStaple_EmptyCert(t *testing.T) {
	s := NewOCSPStapler()
	cert := &tls.Certificate{}
	_, err := s.GetOCSPStaple(t.Context(), cert)
	if err == nil {
		t.Error("GetOCSPStaple(empty cert) should return error")
	}
}

func TestNeedsRefresh_NilCert(t *testing.T) {
	s := NewOCSPStapler()
	if s.NeedsRefresh(nil) {
		t.Error("NeedsRefresh(nil) should return false")
	}
}

func TestNeedsRefresh_EmptyCert(t *testing.T) {
	s := NewOCSPStapler()
	if s.NeedsRefresh(&tls.Certificate{}) {
		t.Error("NeedsRefresh(empty) should return false")
	}
}

func TestRunRefreshCycle_NilCert(t *testing.T) {
	s := NewOCSPStapler()
	err := s.RunRefreshCycle(t.Context(), nil)
	if err == nil {
		t.Error("RunRefreshCycle(nil) should return error")
	}
}

func TestRunRefreshCycle_EmptyCert(t *testing.T) {
	s := NewOCSPStapler()
	err := s.RunRefreshCycle(t.Context(), &tls.Certificate{})
	if err == nil {
		t.Error("RunRefreshCycle(empty) should return error")
	}
}

func TestStapleOCSPToConfig_NilConfig(t *testing.T) {
	s := NewOCSPStapler()
	err := StapleOCSPToConfig(nil, s)
	if err == nil {
		t.Error("StapleOCSPToConfig(nil, s) should return error")
	}
}

func TestStapleOCSPToConfig_NilStapler(t *testing.T) {
	config := &tls.Config{}
	err := StapleOCSPToConfig(config, nil)
	if err == nil {
		t.Error("StapleOCSPToConfig(config, nil) should return error")
	}
}

func TestCompressDecompressData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"empty slice", []byte{}},
		{"small data", []byte("hello world")},
		{"larger data", bytes.Repeat([]byte("test data for compression "), 100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed, err := compressData(tt.data)
			if err != nil {
				t.Fatalf("compressData() error = %v", err)
			}

			if len(tt.data) == 0 {
				if len(compressed) != len(tt.data) {
					t.Errorf("compressData(empty) returned non-empty")
				}
				return
			}

			decompressed, err := decompressData(compressed)
			if err != nil {
				t.Fatalf("decompressData() error = %v", err)
			}

			if !bytes.Equal(decompressed, tt.data) {
				t.Errorf("roundtrip failed: got %d bytes, want %d bytes", len(decompressed), len(tt.data))
			}
		})
	}
}

func TestPrepareCacheEntry_NoCompression(t *testing.T) {
	s := NewOCSPStapler()
	data := []byte("test response data")
	nextUpdate := time.Now().Add(24 * time.Hour)

	entry := s.prepareCacheEntry(t.Context(), data, nextUpdate)
	if entry == nil {
		t.Fatal("prepareCacheEntry() returned nil")
	}
	if entry.isCompressed {
		t.Error("entry should not be compressed")
	}
	if !bytes.Equal(entry.response, data) {
		t.Error("response data mismatch")
	}
}

func TestPrepareCacheEntry_WithCompression(t *testing.T) {
	s := NewOCSPStapler(WithCompression())
	data := bytes.Repeat([]byte("test response data "), 50)
	nextUpdate := time.Now().Add(24 * time.Hour)

	entry := s.prepareCacheEntry(t.Context(), data, nextUpdate)
	if entry == nil {
		t.Fatal("prepareCacheEntry() returned nil")
	}
	if !entry.isCompressed {
		t.Error("entry should be compressed")
	}
	if entry.originalSize != len(data) {
		t.Errorf("originalSize = %d, want %d", entry.originalSize, len(data))
	}
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

	if _, ok := s.cache["expired"]; ok {
		t.Error("expired entry should have been removed")
	}
	if _, ok := s.cache["valid"]; !ok {
		t.Error("valid entry should still exist")
	}
}

func TestRunRefreshAll_NotSchedulerManaged(t *testing.T) {
	s := NewOCSPStapler()
	// No items in cache, should succeed
	err := s.RunRefreshAll(t.Context())
	if err != nil {
		t.Errorf("RunRefreshAll() = %v, want nil", err)
	}
}

func TestRunRefreshAll_SchedulerManaged(t *testing.T) {
	s := NewOCSPStapler()
	_ = s.RegisterRefreshAllSchedulerFunc()

	err := s.RunRefreshAll(t.Context())
	if !errors.Is(err, ErrSchedulerManaged) {
		t.Errorf("RunRefreshAll() = %v, want %v", err, ErrSchedulerManaged)
	}
}

func TestRegisterRefreshAllSchedulerFunc(t *testing.T) {
	s := NewOCSPStapler()
	fn := s.RegisterRefreshAllSchedulerFunc()
	if fn == nil {
		t.Fatal("RegisterRefreshAllSchedulerFunc() returned nil")
	}

	if !s.schedulerRefreshAllRegistered.Load() {
		t.Error("schedulerRefreshAllRegistered should be true")
	}
}
