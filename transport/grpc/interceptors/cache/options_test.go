// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"
)

// compression is used by TestWithCompressionPreset and TestMethodConfig_HasCompressor

func TestMethodConfig_Compressor(t *testing.T) {
	mc := &MethodConfig{}
	c := mc.Compressor()
	if c == nil {
		t.Fatal("should return noop compressor")
	}
}

func TestMethodConfig_HasCompressor(t *testing.T) {
	mc := &MethodConfig{}
	if mc.HasCompressor() {
		t.Fatal("should not have compressor")
	}

	mc.compressor = compression.NewCompressor(1024, 0, 6)
	if !mc.HasCompressor() {
		t.Fatal("should have compressor")
	}
}

func TestWithMethod(t *testing.T) {
	opts := newOptions(WithMethod("/svc/Get", "proto"))
	if _, ok := opts.methods["/svc/get"]; !ok {
		t.Fatal("method should be registered (lowercased)")
	}
}

func TestWithMethod_Empty(t *testing.T) {
	opts := newOptions(WithMethod("", "proto"))
	if len(opts.methods) != 0 {
		t.Fatal("empty method should be skipped")
	}
}

func TestWithDefaultTTL(t *testing.T) {
	opts := newOptions(WithDefaultTTL(10 * time.Minute))
	if opts.cacheTTL != 10*time.Minute {
		t.Fatalf("cacheTTL = %v", opts.cacheTTL)
	}
}

func TestWithDefaultTTL_Invalid(t *testing.T) {
	opts := newOptions(WithDefaultTTL(0))
	if opts.cacheTTL != DefaultTTL {
		t.Fatalf("cacheTTL = %v, want DefaultTTL", opts.cacheTTL)
	}
}

func TestWithCacheHeaders(t *testing.T) {
	opts := newOptions(WithCacheHeaders(false))
	if opts.cacheHeadersEnabled {
		t.Fatal("should be disabled")
	}
}

func TestWithCompressionPreset(t *testing.T) {
	opts := newOptions(WithCompressionPreset(compression.PresetFast))
	if opts.serializer == nil {
		t.Fatal("serializer should be set")
	}
}

func TestHeaderConstants(t *testing.T) {
	if HeaderKey == "" || HeaderHit == "" || HeaderMiss == "" {
		t.Fatal("header constants should not be empty")
	}
}
