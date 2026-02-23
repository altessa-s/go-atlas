// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"net/netip"
	"testing"
)

func BenchmarkIsPrivate(b *testing.B) {
	addr := netip.MustParseAddr("10.0.0.1")
	for b.Loop() {
		IsPrivate(addr)
	}
}

func BenchmarkIsPrivate_Public(b *testing.B) {
	addr := netip.MustParseAddr("8.8.8.8")
	for b.Loop() {
		IsPrivate(addr)
	}
}

func BenchmarkExtractor_Extract_UntrustedPeer(b *testing.B) {
	ext, _ := NewExtractor()
	peerIP := netip.MustParseAddr("8.8.8.8")
	headers := &benchHeaders{}
	ctx := b.Context()

	for b.Loop() {
		ext.Extract(ctx, peerIP, headers)
	}
}

func BenchmarkExtractor_Extract_WithHeaders(b *testing.B) {
	ext, _ := NewExtractor()
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &benchHeaders{data: map[string][]string{
		HeaderXForwardedFor: {"203.0.114.1"},
	}}
	ctx := b.Context()

	for b.Loop() {
		ext.Extract(ctx, peerIP, headers)
	}
}

type benchHeaders struct {
	data map[string][]string
}

func (m *benchHeaders) GetHeader(name string) []string {
	return m.data[name]
}
