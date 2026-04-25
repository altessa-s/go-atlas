// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"net/url"
	"testing"
)

func BenchmarkNew_WithProxyURL(b *testing.B) {
	u, err := url.Parse("http://proxy.local:3128")
	if err != nil {
		b.Fatalf("url.Parse: %v", err)
	}
	for b.Loop() {
		_ = New(WithProxyURL(u))
	}
}
