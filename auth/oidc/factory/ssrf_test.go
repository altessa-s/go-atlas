// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

func TestRejectsProxyWithSSRFProtection(t *testing.T) {
	t.Parallel()
	proxy := &proxyconfig.Config{Mode: proxyconfig.ModeURL, URL: "http://proxy.example:8080"}
	_, err := New(&authconfig.OIDC{Proxy: proxy}).buildProviderOptions(t.Context())
	require.ErrorIs(t, err, httpclient.ErrSSRFUnsafeTransport)
}
