// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import "net/http"

// HTTPClientSetter is the optional interface a pluggable sub-component
// (revocation loader, fetcher, sub-resource client, …) implements to
// opt into receiving a parent's shared *http.Client.
//
// It exists so consumers higher up the stack — for example
// [auth/oidc.Provider] when wiring a [auth/oidc.URLRevocationLoader] —
// can hand their already-configured client (built via [New] from a
// proxy resolver, retry policy, breaker, etc.) to inner components
// without each one reimplementing the wiring or duplicating the
// config-to-options translation.
//
// Implementations own their preserve-vs-overwrite policy. The typical
// pattern is "adopt only when my own field is nil," so an explicit
// caller-supplied client survives the injection:
//
//	func (l *MyLoader) SetHTTPClient(c *http.Client) {
//	    if l.Client == nil {
//	        l.Client = c
//	    }
//	}
//
// Callers walking a slice of dependencies inject via type assertion:
//
//	if setter, ok := dep.(httpclient.HTTPClientSetter); ok {
//	    setter.SetHTTPClient(parentClient)
//	}
type HTTPClientSetter interface {
	SetHTTPClient(*http.Client)
}
