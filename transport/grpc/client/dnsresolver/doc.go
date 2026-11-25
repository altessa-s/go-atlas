// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package dnsresolver resolves the dns targets of gRPC client connections and
// keeps their address lists current.
//
// [NewBuilder] returns a [resolver.Builder] for the dns scheme. Install it on
// a connection with [grpc.WithResolvers]: it never registers itself globally,
// so other gRPC users of the process keep grpc-go's resolver.
// transport/grpc/client installs it when WithDNSResolver is used.
//
// While lookups succeed, a resolver re-resolves its host every refresh
// interval, shortened at random by up to the refresh jitter, so addresses
// that appear or disappear (the pods behind a headless Kubernetes service,
// for example) reach the connection without waiting for a connection to fail.
// ResolveNow, which gRPC calls when a subchannel fails, brings the next lookup
// forward, though never sooner than the minimum interval after the previous
// one.
//
// A and AAAA lookups are independent. The resolver publishes usable
// answers and retains addresses for a family whose lookup fails. It
// submits an empty state only if both families conclusively return no
// addresses. Round-robin removes the old subchannels on an empty state,
// which lets a returning server get a new subchannel on the same address.
// Other policies may keep their subchannels. Failed and empty lookups
// are retried with capped exponential backoff and jitter.
//
// An IP literal target resolves to itself without polling. Service configs
// published in DNS TXT records are not looked up: the connection always uses
// its default service config.
//
// The resolver logs only transitions (lookups start failing, lookups recover)
// and, given a [metrics.Meter], counts lookups by result and reports the
// aggregate address count across active polling resolvers.
package dnsresolver
