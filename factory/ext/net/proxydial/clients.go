// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/url"

	netproxy "github.com/altessa-s/go-atlas-net/proxyconfig"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
)

// ClientOptions folds the proxy Mode switch over a client's option
// constructors, so every client factory (factory/ext/net/httpclient,
// factory/transport/grpc/client) materializes the configuration the same way.
//
// A nil receiver or empty Mode returns (nil, nil) so the client keeps its own
// default proxy policy. An invalid URL in Mode url surfaces as an error here
// rather than at request or dial time.
func ClientOptions[O any](
	p *proxyconfig.Config,
	withoutProxy func() O,
	withProxyURL func(*url.URL) O,
	withProxy func(string, int, *url.Userinfo) O,
) ([]O, error) {
	u, err := p.URLValue()
	if err != nil {
		return nil, err
	}
	if u != nil && p.Mode == netproxy.ModeHost {
		return []O{withProxy(p.Host, p.Port, u.User)}, nil
	}
	if u != nil {
		return []O{withProxyURL(u)}, nil
	}
	if p != nil && p.Mode == netproxy.ModeNone {
		return []O{withoutProxy()}, nil
	}
	return nil, nil
}
