// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/proxydial/factory"
	"testing"
)

func BenchmarkProxy_HTTPClientOptions_Passthrough(b *testing.B) {
	cfg := config.Proxy{}
	for b.Loop() {
		_, _ = factory.HTTPClientOptions(&cfg)
	}
}

func BenchmarkProxy_HTTPClientOptions_None(b *testing.B) {
	cfg := config.Proxy{Mode: config.ProxyModeNone}
	for b.Loop() {
		_, _ = factory.HTTPClientOptions(&cfg)
	}
}

func BenchmarkProxy_HTTPClientOptions_URL(b *testing.B) {
	cfg := config.Proxy{Mode: config.ProxyModeURL, URL: "http://proxy.local:3128"}
	for b.Loop() {
		_, _ = factory.HTTPClientOptions(&cfg)
	}
}

func BenchmarkProxy_HTTPClientOptions_Host(b *testing.B) {
	cfg := config.Proxy{
		Mode: config.ProxyModeHost, Host: "proxy.local", Port: 3128,
		Auth: &config.ProxyAuth{Username: "svc", Password: "secret"},
	}
	for b.Loop() {
		_, _ = factory.HTTPClientOptions(&cfg)
	}
}

func BenchmarkProxy_GrpcClientOptions_Passthrough(b *testing.B) {
	cfg := config.Proxy{}
	for b.Loop() {
		_, _ = factory.GRPCClientOptions(&cfg)
	}
}

func BenchmarkProxy_GrpcClientOptions_None(b *testing.B) {
	cfg := config.Proxy{Mode: config.ProxyModeNone}
	for b.Loop() {
		_, _ = factory.GRPCClientOptions(&cfg)
	}
}

func BenchmarkProxy_GrpcClientOptions_URL(b *testing.B) {
	cfg := config.Proxy{Mode: config.ProxyModeURL, URL: "http://proxy.local:3128"}
	for b.Loop() {
		_, _ = factory.GRPCClientOptions(&cfg)
	}
}

func BenchmarkProxy_GrpcClientOptions_Host(b *testing.B) {
	cfg := config.Proxy{
		Mode: config.ProxyModeHost, Host: "proxy.local", Port: 3128,
		Auth: &config.ProxyAuth{Username: "svc", Password: "secret"},
	}
	for b.Loop() {
		_, _ = factory.GRPCClientOptions(&cfg)
	}
}
