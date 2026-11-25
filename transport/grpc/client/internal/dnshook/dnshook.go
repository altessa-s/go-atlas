// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package dnshook lets the tests of the transport/grpc/client tree replace
// the host lookup of the dnsresolver package, which does not export it.
package dnshook

import "context"

// LookupFunc resolves a host name to IP address literals, like
// [net.Resolver.LookupHost].
type LookupFunc = func(ctx context.Context, host string) ([]string, error)

// WithLookup returns a dnsresolver.Option that makes the resolvers of a
// builder call lookup instead of DNS. The dnsresolver package sets it during
// initialization; the result is typed any because this package cannot import
// dnsresolver.
var WithLookup func(lookup LookupFunc) any
