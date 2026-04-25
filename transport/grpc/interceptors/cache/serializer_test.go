// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"
)

func TestNewDefaultSerializer(t *testing.T) {
	s := NewDefaultSerializer(nil)
	require.NotNil(t, s, "should not be nil")
	require.NotNil(t, s.compressor, "should have noop compressor")
}

func TestNewDefaultSerializer_WithCompressor(t *testing.T) {
	c := compression.NewCompressor(100, 0, 6)
	s := NewDefaultSerializer(c)
	require.Equal(t, c, s.compressor)
}
