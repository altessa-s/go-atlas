// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"bytes"
	"log/slog"
	"testing"
)

func BenchmarkSPIConstraint_Matches(b *testing.B) {
	c := SPIConstraint{Major: 2, MinMinor: 1}
	v := SPIVersion{Contract: "Provider", Major: 2, Minor: 3}
	for b.Loop() {
		_ = c.Matches(v)
	}
}

func BenchmarkUnwrapSPIVersion(b *testing.B) {
	v := SPIVersion{Contract: "Provider", Major: 2, Minor: 1}
	sym := any(&v)
	for b.Loop() {
		_, _ = unwrapSPIVersion(sym)
	}
}

func BenchmarkNegotiateAll(b *testing.B) {
	mgr := NewManager()
	b.Cleanup(func() { _ = mgr.Close() })

	const pluginCount = 50
	for i := range pluginCount {
		name := "p" + string(rune('A'+i%26)) + string(rune('0'+i/26))
		p := newTestPlugin(name, StateReady)
		p.symbols.Store("Provider", symbolEntry{value: "impl", ok: true})
		ver := SPIVersion{Contract: "Provider", Major: 2, Minor: 1}
		p.symbols.Store("ProviderSPIVersion", symbolEntry{value: &ver, ok: true})
		mgr.plugins[name] = p
	}

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	constraint := SPIConstraint{Major: 2, MinMinor: 0}

	b.ResetTimer()
	for b.Loop() {
		for range NegotiateAll(mgr, "Provider", constraint, logger) {
		}
	}
}
