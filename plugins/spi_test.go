// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSPIConstraint_Matches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		constraint SPIConstraint
		version    SPIVersion
		want       bool
	}{
		{
			name:       "exact match",
			constraint: SPIConstraint{Major: 2, MinMinor: 1},
			version:    SPIVersion{Major: 2, Minor: 1},
			want:       true,
		},
		{
			name:       "higher minor satisfies",
			constraint: SPIConstraint{Major: 2, MinMinor: 1},
			version:    SPIVersion{Major: 2, Minor: 3},
			want:       true,
		},
		{
			name:       "lower minor rejected",
			constraint: SPIConstraint{Major: 2, MinMinor: 2},
			version:    SPIVersion{Major: 2, Minor: 1},
			want:       false,
		},
		{
			name:       "major mismatch rejected",
			constraint: SPIConstraint{Major: 2, MinMinor: 0},
			version:    SPIVersion{Major: 3, Minor: 0},
			want:       false,
		},
		{
			name:       "zero constraint matches zero version",
			constraint: SPIConstraint{Major: 0, MinMinor: 0},
			version:    SPIVersion{Major: 0, Minor: 0},
			want:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.constraint.Matches(tc.version))
		})
	}
}

func TestUnwrapSPIVersion(t *testing.T) {
	t.Parallel()

	v := SPIVersion{Contract: "Foo", Major: 1, Minor: 2}
	vPtr := &v

	tests := []struct {
		name      string
		sym       any
		wantValid bool
		wantVer   SPIVersion
	}{
		{
			name:      "*SPIVersion",
			sym:       &v,
			wantValid: true,
			wantVer:   SPIVersion{Contract: "Foo", Major: 1, Minor: 2},
		},
		{
			name:      "**SPIVersion",
			sym:       &vPtr,
			wantValid: true,
			wantVer:   SPIVersion{Contract: "Foo", Major: 1, Minor: 2},
		},
		{
			name:      "nil *SPIVersion",
			sym:       (*SPIVersion)(nil),
			wantValid: false,
		},
		{
			name:      "nil **SPIVersion",
			sym:       (**SPIVersion)(nil),
			wantValid: false,
		},
		{
			name:      "wrong type",
			sym:       "not a version",
			wantValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, valid := unwrapSPIVersion(tc.sym)
			assert.Equal(t, tc.wantValid, valid)
			if tc.wantValid {
				assert.Equal(t, tc.wantVer, got)
			}
		})
	}
}

func TestNegotiateAll(t *testing.T) {
	t.Parallel()

	// Build a manager with three ready plugins, each with a cached Provider symbol.
	mgr := NewManager()
	t.Cleanup(func() { _ = mgr.Close() })

	// p1: versioned, compatible (major=2, minor=1).
	p1 := newTestPlugin("p1", StateReady)
	p1.symbols.Store("Provider", symbolEntry{value: "provider1", ok: true})
	ver1 := SPIVersion{Contract: "Provider", Major: 2, Minor: 1}
	p1.symbols.Store("ProviderSPIVersion", symbolEntry{value: &ver1, ok: true})

	// p2: versioned, incompatible major.
	p2 := newTestPlugin("p2", StateReady)
	p2.symbols.Store("Provider", symbolEntry{value: "provider2", ok: true})
	ver2 := SPIVersion{Contract: "Provider", Major: 3, Minor: 0}
	p2.symbols.Store("ProviderSPIVersion", symbolEntry{value: &ver2, ok: true})

	// p3: unversioned — should be included (backwards compat).
	p3 := newTestPlugin("p3", StateReady)
	p3.symbols.Store("Provider", symbolEntry{value: "provider3", ok: true})

	// p4: versioned but minor too low.
	p4 := newTestPlugin("p4", StateReady)
	p4.symbols.Store("Provider", symbolEntry{value: "provider4", ok: true})
	ver4 := SPIVersion{Contract: "Provider", Major: 2, Minor: 0}
	p4.symbols.Store("ProviderSPIVersion", symbolEntry{value: &ver4, ok: true})

	// p5: malformed version symbol type.
	p5 := newTestPlugin("p5", StateReady)
	p5.symbols.Store("Provider", symbolEntry{value: "provider5", ok: true})
	p5.symbols.Store("ProviderSPIVersion", symbolEntry{value: "not-a-version", ok: true})

	mgr.plugins["p1"] = p1
	mgr.plugins["p2"] = p2
	mgr.plugins["p3"] = p3
	mgr.plugins["p4"] = p4
	mgr.plugins["p5"] = p5

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	constraint := SPIConstraint{Major: 2, MinMinor: 1}
	var matched []string
	for _, sym := range NegotiateAll(mgr, "Provider", constraint, logger) {
		s, ok := sym.(string)
		require.True(t, ok)
		matched = append(matched, s)
	}

	// p1 (compatible) and p3 (unversioned) should be included.
	assert.Contains(t, matched, "provider1", "compatible versioned plugin")
	assert.Contains(t, matched, "provider3", "unversioned plugin (backwards compat)")
	assert.NotContains(t, matched, "provider2", "major mismatch should be excluded")
	assert.NotContains(t, matched, "provider4", "minor too low should be excluded")
	assert.NotContains(t, matched, "provider5", "malformed version should be excluded")
	assert.Len(t, matched, 2)

	// Warnings should be logged for excluded plugins.
	logOutput := buf.String()
	assert.Contains(t, logOutput, "does not satisfy host constraint")
	assert.Contains(t, logOutput, "malformed SPI version symbol")
}

func TestNegotiateAll_EmptyManager(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	t.Cleanup(func() { _ = mgr.Close() })

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	constraint := SPIConstraint{Major: 1, MinMinor: 0}

	count := 0
	for range NegotiateAll(mgr, "Provider", constraint, logger) {
		count++
	}
	assert.Equal(t, 0, count)
}
