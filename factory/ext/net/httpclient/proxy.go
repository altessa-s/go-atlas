// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	proxyfactory "github.com/altessa-s/go-atlas/factory/ext/net/proxydial"
)

// ProxyOptions materializes the proxy configuration into net/http/client
// options ready to be passed to httpclient.New.
//
// A nil receiver or empty Mode returns (nil, nil) so the client keeps
// its default policy: SSRF protection disables environment proxies. Explicit
// proxy modes work with SSRF protection on: the dial to the proxy is guarded
// (allow a private proxy network with httpclient.WithSSRFAllowedCIDRs) and the
// proxy resolves the target hosts.
//
// An invalid URL in Mode url surfaces as an error here rather than at
// request time.
func ProxyOptions(p *proxyconfig.Config) ([]httpclient.Option, error) {
	return proxyfactory.ClientOptions(p, httpclient.WithoutProxy, httpclient.WithProxyURL, httpclient.WithProxy)
}
