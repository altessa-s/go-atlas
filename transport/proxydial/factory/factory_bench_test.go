// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/altessa-s/go-atlas/config"
)

func BenchmarkDialerBuilder_New(b *testing.B) {
	cfg := &config.HTTPProxy{Mode: config.HTTPProxyModeURL, URL: "https://proxy.example.com:8443"}
	for b.Loop() {
		New(cfg)
	}
}

func BenchmarkDialerBuilder_Build_URL(b *testing.B) {
	cfg := &config.HTTPProxy{Mode: config.HTTPProxyModeURL, URL: "https://proxy.example.com:8443"}
	builder := New(cfg)
	b.ResetTimer()
	for b.Loop() {
		_, _ = builder.Build()
	}
}

func BenchmarkDialerBuilder_Build_Host(b *testing.B) {
	cfg := &config.HTTPProxy{
		Mode: config.HTTPProxyModeHost,
		Host: "proxy.example.com",
		Port: 8443,
		Auth: &config.HTTPProxyAuth{Username: "svc", Password: config.Secret("hunter2")},
	}
	builder := New(cfg)
	b.ResetTimer()
	for b.Loop() {
		_, _ = builder.Build()
	}
}

func BenchmarkDialerBuilder_Build_Disabled(b *testing.B) {
	builder := New(nil)
	b.ResetTimer()
	for b.Loop() {
		_, _ = builder.Build()
	}
}
