// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import "testing"

func BenchmarkHTTPProxy_ClientOptions_Passthrough(b *testing.B) {
	cfg := HTTPProxy{}
	for b.Loop() {
		_, _ = cfg.ClientOptions()
	}
}

func BenchmarkHTTPProxy_ClientOptions_None(b *testing.B) {
	cfg := HTTPProxy{Mode: HTTPProxyModeNone}
	for b.Loop() {
		_, _ = cfg.ClientOptions()
	}
}

func BenchmarkHTTPProxy_ClientOptions_URL(b *testing.B) {
	cfg := HTTPProxy{Mode: HTTPProxyModeURL, URL: "http://proxy.local:3128"}
	for b.Loop() {
		_, _ = cfg.ClientOptions()
	}
}

func BenchmarkHTTPProxy_ClientOptions_Host(b *testing.B) {
	cfg := HTTPProxy{
		Mode: HTTPProxyModeHost, Host: "proxy.local", Port: 3128,
		Auth: &HTTPProxyAuth{Username: "svc", Password: "secret"},
	}
	for b.Loop() {
		_, _ = cfg.ClientOptions()
	}
}
