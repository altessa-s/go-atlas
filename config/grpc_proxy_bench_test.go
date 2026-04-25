// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import "testing"

func BenchmarkGrpcProxy_ClientOptions_Passthrough(b *testing.B) {
	cfg := GrpcProxy{}
	for b.Loop() {
		_, _ = cfg.ClientOptions()
	}
}

func BenchmarkGrpcProxy_ClientOptions_None(b *testing.B) {
	cfg := GrpcProxy{Mode: GrpcProxyModeNone}
	for b.Loop() {
		_, _ = cfg.ClientOptions()
	}
}

func BenchmarkGrpcProxy_ClientOptions_URL(b *testing.B) {
	cfg := GrpcProxy{Mode: GrpcProxyModeURL, URL: "http://proxy.local:3128"}
	for b.Loop() {
		_, _ = cfg.ClientOptions()
	}
}

func BenchmarkGrpcProxy_ClientOptions_Host(b *testing.B) {
	cfg := GrpcProxy{
		Mode: GrpcProxyModeHost, Host: "proxy.local", Port: 3128,
		Auth: &GrpcProxyAuth{Username: "svc", Password: "secret"},
	}
	for b.Loop() {
		_, _ = cfg.ClientOptions()
	}
}
