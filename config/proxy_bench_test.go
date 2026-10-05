// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config"
)

func BenchmarkProxyValidate(b *testing.B) {
	p := config.Proxy{Mode: config.ProxyModeURL, URL: "http://proxy:3128"}
	for b.Loop() {
		_ = p.Validate()
	}
}
