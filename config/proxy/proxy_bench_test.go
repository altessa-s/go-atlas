// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxyconfig_test

import (
	"testing"

	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
)

func BenchmarkProxyValidate(b *testing.B) {
	p := proxyconfig.Config{Mode: proxyconfig.ModeURL, URL: "http://proxy:3128"}
	for b.Loop() {
		_ = p.Validate()
	}
}
