// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package dnsresolver resolves the dns targets of gRPC client connections and
// keeps their address lists current.
//
// [NewBuilder] returns a [resolver.Builder] for the dns scheme. Install it on
// a connection with [grpc.WithResolvers]: it never registers itself globally,
// so other gRPC users of the process keep grpc-go's resolver.
// transport/grpc/client installs it by default.
//
// While lookups succeed, a resolver re-resolves its host every refresh
// interval, shortened at random by up to the refresh jitter, so addresses
// that appear or disappear (the pods behind a headless Kubernetes service,
// for example) reach the connection without waiting for a connection to fail.
// ResolveNow, which gRPC calls when a subchannel fails, brings the next lookup
// forward, though never sooner than the minimum interval after the previous
// one.
//
// A lookup that fails or finds no address is reported to the connection and
// retried with exponential backoff and jitter under a cap of a few seconds,
// so a client sees the addresses of a server that returns after a full outage
// within that cap. A failed lookup (a timeout or SERVFAIL, also of only one of
// its queries) keeps the connection's addresses. An answer that no address
// exists (NXDOMAIN or no records) drops them, which closes their subchannels:
// a server that returns, even on its old address, is then connected at once
// instead of after the connection backoff of its old subchannel.
//
// An IP literal target resolves to itself without polling. Service configs
// published in DNS TXT records are not looked up: the connection always uses
// its default service config.
//
// The resolver logs only transitions (lookups start failing, lookups recover)
// and, given a [metrics.Meter], counts lookups by result and reports the
// address count per target.
package dnsresolver
