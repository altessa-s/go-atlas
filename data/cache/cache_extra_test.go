// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
)

func TestWithSerializer(t *testing.T) {
	s := &serializer.JSON{}
	c := New(newMockProvider(), WithSerializer(s))
	if c == nil {
		t.Fatal("New with WithSerializer returned nil")
	}
}

func TestWithTtl(t *testing.T) {
	c := New(newMockProvider(), WithTtl(5*time.Minute))
	if c == nil {
		t.Fatal("New with WithTtl returned nil")
	}
}

func TestWithTtl_ZeroIgnored(t *testing.T) {
	opts := newOptions(WithTtl(0))
	if opts.ttl != DefaultTTL {
		t.Errorf("zero TTL should be ignored, got %v", opts.ttl)
	}
}

func TestWithTtl_NegativeIgnored(t *testing.T) {
	opts := newOptions(WithTtl(-1 * time.Second))
	if opts.ttl != DefaultTTL {
		t.Errorf("negative TTL should be ignored, got %v", opts.ttl)
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	if opts.ttl != DefaultTTL {
		t.Errorf("default TTL = %v, want %v", opts.ttl, DefaultTTL)
	}
	if opts.serializer == nil {
		t.Error("default serializer should not be nil")
	}
}

func TestSave_WithCustomTTL(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithTtl(10*time.Minute))

	err := c.Save(t.Context(), "k", "v", 5*time.Minute)
	if err != nil {
		t.Fatalf("Save with custom TTL: %v", err)
	}
}

func TestSave_WithNoTTL(t *testing.T) {
	p := newMockProvider()
	c := New(p)

	err := c.Save(t.Context(), "k", "v", NoTTL)
	if err != nil {
		t.Fatalf("Save with NoTTL: %v", err)
	}
}
