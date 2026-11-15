// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"fmt"
	"net/url"

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
		u, err := url.Parse(p.URL.Expose())
		if err != nil {
			// url.Parse errors quote the URL, credentials included.
			return nil, errors.New("proxy: url is not a valid URL")
		}
		return []O{withProxyURL(u)}, nil
	case proxyconfig.ModeHost:
		return []O{withProxy(p.Host, p.Port, userinfo(p.Auth))}, nil
	default:
		return nil, fmt.Errorf("proxy: unknown mode %q", p.Mode)
	}
}
