// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for materializing a
// [github.com/altessa-s/go-atlas/config.HTTPProxy] into a
// [github.com/altessa-s/go-atlas/transport/proxydial.DialContextFunc]
// suitable for any client that needs raw-TCP-through-proxy: SMTP
// (go-mail's WithDialContextFunc), SOAP, gRPC's WithContextDialer,
// or any custom protocol that does not go through net/http.
//
// [DialerBuilder] follows the same fluent shape as the rest of
// go-atlas (vault, oidc, broker factories):
//
//	dial, err := factory.New(cfg.Proxy).
//	    UseLogger(logger).
//	    UseProxyTLSConfig(tlsCfg).
//	    Build()
//	if err != nil { return err }
//	if dial != nil {
//	    mailOpts = append(mailOpts, mail.WithDialContextFunc(dial))
//	}
//
// A nil cfg, an empty Mode, or
// [github.com/altessa-s/go-atlas/config.HTTPProxyModeNone] yields
// (nil, nil) — caller treats that as "use a direct dial" and skips
// wiring a custom dialer entirely.
//
// HTTP and gRPC clients should keep using
// [github.com/altessa-s/go-atlas/config.HTTPProxy.ClientOptions];
// this factory targets consumers that have no httpclient/grpcclient
// in the picture.
package factory
