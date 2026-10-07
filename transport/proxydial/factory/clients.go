// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"net/url"

	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// HTTPClientOptions materializes the proxy configuration into a slice
// of transport/http/client options ready to be passed to httpclient.New.
//
// A nil receiver or empty Mode returns (nil, nil) so the client keeps
// the http.ProxyFromEnvironment default (HTTP_PROXY / HTTPS_PROXY /
// NO_PROXY).
//
// An invalid URL in Mode url surfaces as an error here rather than at
// request time.
func HTTPClientOptions(p *proxyconfig.Config) ([]httpclient.Option, error) {
	return proxyClientOptions(p,
		httpclient.WithoutProxy, httpclient.WithProxyURL, httpclient.WithProxy)
}

// GRPCClientOptions materializes the proxy configuration into a slice
// of transport/grpc/client options ready to be passed to grpcclient.New.
//
// A nil receiver or empty Mode returns (nil, nil) so the client keeps
// grpc-go's own HTTPS_PROXY / HTTP_PROXY / NO_PROXY env default.
//
// An invalid URL in Mode url surfaces as an error here rather than at
// dial time.
func GRPCClientOptions(p *proxyconfig.Config) ([]grpcclient.Option, error) {
	return proxyClientOptions(p,
		grpcclient.WithoutProxy, grpcclient.WithProxyURL, grpcclient.WithProxy)
}

// proxyClientOptions folds the Mode switch shared by HTTPClientOptions
// and GRPCClientOptions over the transport-specific option
// constructors.
func proxyClientOptions[O any](
	p *proxyconfig.Config,
	withoutProxy func() O,
	withProxyURL func(*url.URL) O,
	withProxy func(string, int, *url.Userinfo) O,
) ([]O, error) {
	if p == nil {
		return nil, nil
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	switch p.Mode {
	case "":
		return nil, nil
	case proxyconfig.ModeNone:
		return []O{withoutProxy()}, nil
	case proxyconfig.ModeURL:
		u, err := url.Parse(p.URL)
		if err != nil {
			return nil, fmt.Errorf("proxy: parse url: %w", err)
		}
		return []O{withProxyURL(u)}, nil
	case proxyconfig.ModeHost:
		return []O{withProxy(p.Host, p.Port, userinfo(p.Auth))}, nil
	default:
		return nil, fmt.Errorf("proxy: unknown mode %q", p.Mode)
	}
}
