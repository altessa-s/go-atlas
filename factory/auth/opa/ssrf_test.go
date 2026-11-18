// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	netproxy "github.com/altessa-s/go-atlas-net/proxyconfig"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
)

func TestRejectsProxyWithSSRFProtection(t *testing.T) {
	t.Parallel()
	proxy := &proxyconfig.Config{Mode: netproxy.ModeURL, URL: "http://proxy.example:8080"}
	b := New(&authconfig.OPA{GitLab: &authconfig.OPAGitLab{Proxy: proxy}})
	_, err := b.buildGitLabSource()
	require.ErrorIs(t, err, httpclient.ErrSSRFUnsafeTransport)
	_, err = b.resolveS3Client(t.Context(), &authconfig.OPAS3{Proxy: proxy})
	require.ErrorIs(t, err, httpclient.ErrSSRFUnsafeTransport)
}
