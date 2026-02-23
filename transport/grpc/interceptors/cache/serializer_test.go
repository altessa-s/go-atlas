// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"
)

func TestNewDefaultSerializer(t *testing.T) {
	s := NewDefaultSerializer(nil)
	if s == nil {
		t.Fatal("should not be nil")
	}
	if s.compressor == nil {
		t.Fatal("should have noop compressor")
	}
}

func TestNewDefaultSerializer_WithCompressor(t *testing.T) {
	c := compression.NewCompressor(100, 0, 6)
	s := NewDefaultSerializer(c)
	if s.compressor != c {
		t.Fatal("should use provided compressor")
	}
}
