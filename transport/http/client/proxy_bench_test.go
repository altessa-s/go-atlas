// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkNew_WithProxyURL(b *testing.B) {
	u, err := url.Parse("http://proxy.local:3128")
	require.NoError(b, err)
	for b.Loop() {
		_ = New(WithoutSSRFProtection(), WithProxyURL(u))
	}
}
