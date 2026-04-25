// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"
)

// compression is used by TestWithCompressionPreset and TestMethodConfig_HasCompressor

func TestMethodConfig_Compressor(t *testing.T) {
	mc := &MethodConfig{}
	c := mc.Compressor()
	require.NotNil(t, c, "should return noop compressor")
}

func TestMethodConfig_HasCompressor(t *testing.T) {
	mc := &MethodConfig{}
	require.False(t, mc.HasCompressor(), "should not have compressor")

	mc.compressor = compression.NewCompressor(1024, 0, 6)
	require.True(t, mc.HasCompressor(), "should have compressor")
}

func TestWithMethod(t *testing.T) {
	opts := newOptions(WithMethod("/svc/Get", "proto"))
	_, ok := opts.methods["/svc/get"]
	require.True(t, ok, "method should be registered (lowercased)")
}

func TestWithMethod_Empty(t *testing.T) {
	opts := newOptions(WithMethod("", "proto"))
	require.Len(t, opts.methods, 0)
}

func TestWithDefaultTTL(t *testing.T) {
	opts := newOptions(WithDefaultTTL(10 * time.Minute))
	require.Equal(t, 10*time.Minute, opts.cacheTTL)
}

func TestWithDefaultTTL_Invalid(t *testing.T) {
	opts := newOptions(WithDefaultTTL(0))
	require.Equal(t, DefaultTTL, opts.cacheTTL)
}

func TestWithCacheHeaders(t *testing.T) {
	opts := newOptions(WithCacheHeaders(false))
	require.False(t, opts.cacheHeadersEnabled, "should be disabled")
}

func TestWithCompressionPreset(t *testing.T) {
	opts := newOptions(WithCompressionPreset(compression.PresetFast))
	require.NotNil(t, opts.serializer, "serializer should be set")
}

func TestHeaderConstants(t *testing.T) {
	require.NotEqual(t, "", HeaderKey)
	require.NotEqual(t, "", HeaderHit)
	require.NotEqual(t, "", HeaderMiss)
}
