// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	proxyfactory "github.com/altessa-s/go-atlas/factory/ext/net/proxydial"
	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
)

// ProxyOptions materializes the proxy configuration into
// transport/grpc/client options ready to be passed to grpcclient.New.
//
// A nil receiver or empty Mode returns (nil, nil) so the client keeps
// grpc-go's own HTTPS_PROXY / HTTP_PROXY / NO_PROXY env default.
//
// An invalid URL in Mode url surfaces as an error here rather than at
// dial time.
func ProxyOptions(p *proxyconfig.Config) ([]grpcclient.Option, error) {
	return proxyfactory.ClientOptions(p, grpcclient.WithoutProxy, grpcclient.WithProxyURL, grpcclient.WithProxy)
}
