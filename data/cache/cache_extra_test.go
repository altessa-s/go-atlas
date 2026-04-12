// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
)

func TestWithSerializer(t *testing.T) {
	s := &serializer.JSON{}
	c := New(newMockProvider(), WithSerializer(s))
	require.NotNil(t, c)
}

func TestWithTtl(t *testing.T) {
	c := New(newMockProvider(), WithTtl(5*time.Minute))
	require.NotNil(t, c)
}

func TestWithTtl_ZeroIgnored(t *testing.T) {
	opts := newOptions(WithTtl(0))
	require.Equal(t, DefaultTTL, opts.ttl, "zero TTL should be ignored")
}

func TestWithTtl_NegativeIgnored(t *testing.T) {
	opts := newOptions(WithTtl(-1 * time.Second))
	require.Equal(t, DefaultTTL, opts.ttl, "negative TTL should be ignored")
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	require.Equal(t, DefaultTTL, opts.ttl)
	require.NotNil(t, opts.serializer, "default serializer should not be nil")
}

func TestSave_WithCustomTTL(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithTtl(10*time.Minute))

	err := c.Save(t.Context(), "k", "v", 5*time.Minute)
	require.NoError(t, err)
}

func TestSave_WithNoTTL(t *testing.T) {
	p := newMockProvider()
	c := New(p)

	err := c.Save(t.Context(), "k", "v", NoTTL)
	require.NoError(t, err)
}
