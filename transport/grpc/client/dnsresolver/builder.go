// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dnsresolver

import (
	"context"
	"fmt"
	"net"

	"github.com/altessa-s/go-atlas/transport/grpc/client/internal/dnshook"

	"google.golang.org/grpc/resolver"
)

var _ resolver.Builder = (*builder)(nil)

// NewBuilder returns a [resolver.Builder] for the dns scheme. Pass it to one
// connection with [grpc.WithResolvers], where it takes precedence over the
// dns resolver grpc-go registers globally; NewBuilder registers nothing, so
// other connections of the process keep grpc-go's resolver.
//
// Each connection the builder serves gets a resolver of its own that
// resolves the target host until the connection closes it. Targets have the
// forms grpc-go's dns resolver accepts: host:port, dns:///host:port,
// dns://server/host:port (lookups go to the DNS server at server, port 53 by
// default) and IPv4 or bracketed IPv6 literals; a target without a port uses
// port 443. An IP literal resolves to itself once, without polling.
//
// Example:
//
//	conn, err := grpc.NewClient("inventory.default.svc:9000",
//		grpc.WithResolvers(dnsresolver.NewBuilder(dnsresolver.WithRefreshInterval(10*time.Second))),
//		grpc.WithTransportCredentials(creds),
//	)
func NewBuilder(opts ...Option) resolver.Builder {
	o := newOptions(opts...)
	o.normalize()
	return &builder{opts: o, metrics: newBuilderMetrics(o.meter)}
}

// builder creates the resolvers of the connections it is installed on.
type builder struct {
	opts    *options
	metrics *builderMetrics
}

// Build parses the target and starts resolving it. An invalid target or DNS
// server fails the build, and with it the connection.
func (b *builder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	host, port, err := parseTarget(target.Endpoint(), defaultPort)
	if err != nil {
		return nil, fmt.Errorf("dnsresolver: target %q: %w", target.Endpoint(), err)
	}
	if isIP(host) {
		// One address that never changes. An error only means the balancer
		// rejected it, which no later update could change.
		_ = cc.UpdateState(newState([]string{net.JoinHostPort(host, port)}))
		return staticResolver{}, nil
	}
	lookup := b.opts.lookup
	if lookup == nil {
		server, serverErr := dnsServer(target.URL.Host)
		if serverErr != nil {
			return nil, fmt.Errorf("dnsresolver: DNS server %q: %w", target.URL.Host, serverErr)
		}
		lookup = netResolver(server).LookupHost
	}
	addr := net.JoinHostPort(host, port)
	ctx, cancel := context.WithCancel(context.Background())
	r := &dnsResolver{
		cc:         cc,
		lookup:     lookup,
		opts:       b.opts,
		metrics:    b.metrics.forTarget(addr),
		host:       host,
		port:       port,
		target:     addr,
		ctx:        ctx,
		cancel:     cancel,
		resolveNow: make(chan struct{}, 1),
		done:       make(chan struct{}),
	}
	go r.run()
	return r, nil
}

// Scheme returns [Scheme].
func (*builder) Scheme() string {
	return Scheme
}

// staticResolver serves an IP literal target, which never needs resolving.
type staticResolver struct{}

func (staticResolver) ResolveNow(resolver.ResolveNowOptions) {}

func (staticResolver) Close() {}

// withLookup makes the resolvers call lookup instead of DNS.
func withLookup(lookup dnshook.LookupFunc) Option {
	return func(o *options) {
		o.lookup = lookup
	}
}

// Tests elsewhere in the transport/grpc/client tree resolve through
// dnshook.WithLookup without the lookup becoming public API.
func init() {
	dnshook.WithLookup = func(lookup dnshook.LookupFunc) any { return withLookup(lookup) }
}
