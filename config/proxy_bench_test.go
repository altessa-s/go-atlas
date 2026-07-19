// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import "testing"

func BenchmarkProxy_HTTPClientOptions_Passthrough(b *testing.B) {
	cfg := Proxy{}
	for b.Loop() {
		_, _ = cfg.HTTPClientOptions()
	}
}

func BenchmarkProxy_HTTPClientOptions_None(b *testing.B) {
	cfg := Proxy{Mode: ProxyModeNone}
	for b.Loop() {
		_, _ = cfg.HTTPClientOptions()
	}
}

func BenchmarkProxy_HTTPClientOptions_URL(b *testing.B) {
	cfg := Proxy{Mode: ProxyModeURL, URL: "http://proxy.local:3128"}
	for b.Loop() {
		_, _ = cfg.HTTPClientOptions()
	}
}

func BenchmarkProxy_HTTPClientOptions_Host(b *testing.B) {
	cfg := Proxy{
		Mode: ProxyModeHost, Host: "proxy.local", Port: 3128,
		Auth: &ProxyAuth{Username: "svc", Password: "secret"},
	}
	for b.Loop() {
		_, _ = cfg.HTTPClientOptions()
	}
}

func BenchmarkProxy_GrpcClientOptions_Passthrough(b *testing.B) {
	cfg := Proxy{}
	for b.Loop() {
		_, _ = cfg.GrpcClientOptions()
	}
}

func BenchmarkProxy_GrpcClientOptions_None(b *testing.B) {
	cfg := Proxy{Mode: ProxyModeNone}
	for b.Loop() {
		_, _ = cfg.GrpcClientOptions()
	}
}

func BenchmarkProxy_GrpcClientOptions_URL(b *testing.B) {
	cfg := Proxy{Mode: ProxyModeURL, URL: "http://proxy.local:3128"}
	for b.Loop() {
		_, _ = cfg.GrpcClientOptions()
	}
}

func BenchmarkProxy_GrpcClientOptions_Host(b *testing.B) {
	cfg := Proxy{
		Mode: ProxyModeHost, Host: "proxy.local", Port: 3128,
		Auth: &ProxyAuth{Username: "svc", Password: "secret"},
	}
	for b.Loop() {
		_, _ = cfg.GrpcClientOptions()
	}
}
