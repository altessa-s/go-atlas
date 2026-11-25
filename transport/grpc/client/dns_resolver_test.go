// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/transport/grpc/client/dnsresolver"
	"github.com/altessa-s/go-atlas/transport/grpc/client/internal/dnshook"
	"github.com/altessa-s/go-atlas/transport/grpc/client/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
	"google.golang.org/grpc/status"

	client "github.com/altessa-s/go-atlas/transport/grpc/client"
	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// connectAllowance bounds connection setup to a local server, on top of the
// resolver timing a test asserts, on a loaded machine.
const connectAllowance = 2 * time.Second

// backend is a gRPC server on 127.0.0.1 with the health service that counts
// the unary calls it serves.
type backend struct {
	addr  string
	port  string
	calls atomic.Int32
	// stop stops the server; the test cleanup stops it as well.
	stop func()
}

func startHealthBackend(t *testing.T) *backend {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	b := &backend{addr: lis.Addr().String()}
	_, b.port, err = net.SplitHostPort(b.addr)
	require.NoError(t, err)
	srv := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handle grpc.UnaryHandler) (any, error) {
		b.calls.Add(1)
		return handle(ctx, req)
	}))
	healthpb.RegisterHealthServer(srv, grpchealth.NewServer())
	go func() { _ = srv.Serve(lis) }()
	b.stop = srv.Stop
	t.Cleanup(srv.Stop)
	return b
}

// stubDNS answers the client's lookups with the addresses a test sets; no
// address is NXDOMAIN.
type stubDNS struct {
	mu    sync.Mutex
	addrs []string
	calls atomic.Int32
}

func (d *stubDNS) lookup(_ context.Context, host string) ([]string, error) {
	d.calls.Add(1)
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.addrs) == 0 {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return slices.Clone(d.addrs), nil
}

func (d *stubDNS) set(addrs ...string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.addrs = addrs
}

// option makes the client's resolver look up through d.
func (d *stubDNS) option(t *testing.T) dnsresolver.Option {
	t.Helper()
	opt, ok := dnshook.WithLookup(d.lookup).(dnsresolver.Option)
	require.True(t, ok)
	return opt
}

func connect(t *testing.T, target string, opts ...client.Option) *grpc.ClientConn {
	t.Helper()
	c, err := client.New(t.Context(), target, append([]client.Option{client.WithInsecure(), client.WithoutProxy()}, opts...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(context.WithoutCancel(t.Context())) })
	conn, err := c.GetConnection(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { c.ReturnConnection(conn) })
	return conn
}

func check(ctx context.Context, conn *grpc.ClientConn, opts ...grpc.CallOption) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{}, opts...)
	return err
}

// TestDNSResolverRecoversAfterFullOutage reproduces, end to end, a target
// whose servers are all gone (DNS answers NXDOMAIN) and come back. With
// grpc-go's built-in resolver the round_robin balancer rejects the empty
// answer, and the resolver then retries with backoff.DefaultExponential (1s
// growing by 1.6 up to 120s) without listening to ResolveNow, so after an
// outage of a minute RPCs keep failing for tens of seconds after the server
// is back. That slow path is not run here. With the go-atlas resolver the
// first RPC succeeds within the backoff cap of the address returning.
func TestDNSResolverRecoversAfterFullOutage(t *testing.T) {
	t.Parallel()
	server := startHealthBackend(t)
	dns := &stubDNS{}
	const backoffCap = 300 * time.Millisecond
	conn := connect(t, net.JoinHostPort("orders.test", server.port),
		client.WithDNSResolver(dns.option(t), dnsresolver.WithBackoffBaseDelay(20*time.Millisecond), dnsresolver.WithBackoffMaxDelay(backoffCap)))

	err := check(t.Context(), conn)
	require.Equal(t, codes.Unavailable, status.Code(err), "%v", err)
	require.ErrorContains(t, err, "no addresses for host")

	// The outage lasts until the resolver has backed off to its cap.
	require.Eventually(t, func() bool { return dns.calls.Load() >= 10 }, 10*time.Second, 5*time.Millisecond)
	dns.set("127.0.0.1")
	back := time.Now()
	for err := check(t.Context(), conn); err != nil; err = check(t.Context(), conn) {
		require.Less(t, time.Since(back), backoffCap+connectAllowance, "RPCs still fail after the server returned: %v", err)
		time.Sleep(5 * time.Millisecond)
	}
	require.Positive(t, server.calls.Load())
}

// TestDNSResolverRecoversOnTheSameAddress brings a server back on the address
// it had before the outage, as a container restarted in place keeps its pod
// IP. While DNS finds no address the resolver drops the connection's
// addresses, so the returning server gets a new subchannel and the client does
// not wait out the connection backoff that the failed attempts to the old one
// built up: here a minute per attempt.
func TestDNSResolverRecoversOnTheSameAddress(t *testing.T) {
	t.Parallel()
	const podAddr = "10.0.0.1:9000"
	first := startHealthBackend(t)
	// DNS returns the pod address; the dialer maps it to the server that
	// currently serves it.
	var upstream atomic.Pointer[string]
	upstream.Store(&first.addr)
	dial := func(ctx context.Context, addr string) (net.Conn, error) {
		if addr != podAddr {
			return nil, fmt.Errorf("unexpected address %s", addr)
		}
		var dialer net.Dialer
		return dialer.DialContext(ctx, "tcp", *upstream.Load())
	}
	dns := &stubDNS{}
	dns.set("10.0.0.1")
	const backoffCap = 300 * time.Millisecond
	conn := connect(t, "orders.test:9000",
		client.WithDNSResolver(dns.option(t), dnsresolver.WithMinInterval(10*time.Millisecond),
			dnsresolver.WithBackoffBaseDelay(20*time.Millisecond), dnsresolver.WithBackoffMaxDelay(backoffCap)),
		client.WithConnectBackoff(client.WithConnectBackoffBaseDelay(time.Minute), client.WithConnectBackoffMaxDelay(time.Minute),
			client.WithConnectBackoffJitter(0)),
		client.WithDialOptions(grpc.WithContextDialer(dial)))
	require.NoError(t, check(t.Context(), conn, grpc.WaitForReady(true)))

	// The server goes away and DNS finds nothing; the reconnection attempt
	// fails and the old subchannel backs off for a minute.
	dns.set()
	first.stop()
	lookups := dns.calls.Load()
	require.Eventually(t, func() bool { return dns.calls.Load() >= lookups+2 }, 10*time.Second, 5*time.Millisecond)
	require.Equal(t, codes.Unavailable, status.Code(check(t.Context(), conn)))

	second := startHealthBackend(t)
	upstream.Store(&second.addr)
	dns.set("10.0.0.1")
	back := time.Now()
	for err := check(t.Context(), conn); err != nil; err = check(t.Context(), conn) {
		require.Less(t, time.Since(back), backoffCap+connectAllowance, "RPCs still fail after the server returned on its address: %v", err)
		time.Sleep(5 * time.Millisecond)
	}
	require.Positive(t, second.calls.Load())
}

// TestDNSResolverDiscoversScaleUp adds a server to DNS while the existing one
// stays healthy. It receives traffic within one refresh interval; grpc-go's
// built-in resolver resolves again only after a connection fails, so it
// would not see the new server until then.
func TestDNSResolverDiscoversScaleUp(t *testing.T) {
	t.Parallel()
	first, second := startHealthBackend(t), startHealthBackend(t)
	// DNS returns pod addresses on the target port; the dialer maps them to
	// the local servers.
	backends := map[string]string{"10.0.0.1:9000": first.addr, "10.0.0.2:9000": second.addr}
	dial := func(ctx context.Context, addr string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "tcp", backends[addr])
	}
	dns := &stubDNS{}
	dns.set("10.0.0.1")
	const refresh = 200 * time.Millisecond
	conn := connect(t, "orders.test:9000",
		client.WithDNSResolver(dns.option(t), dnsresolver.WithRefreshInterval(refresh)),
		client.WithDialOptions(grpc.WithContextDialer(dial)))
	require.NoError(t, check(t.Context(), conn, grpc.WaitForReady(true)))
	require.Positive(t, first.calls.Load())

	dns.set("10.0.0.1", "10.0.0.2")
	added := time.Now()
	for second.calls.Load() == 0 {
		require.Less(t, time.Since(added), refresh+connectAllowance, "the added server receives no traffic")
		require.NoError(t, check(t.Context(), conn))
		time.Sleep(5 * time.Millisecond)
	}
}

// Every pooled connection resolves through the resolver: orders.test exists
// only in the stub DNS.
func TestDNSResolverInPoolMode(t *testing.T) {
	t.Parallel()
	server := startHealthBackend(t)
	dns := &stubDNS{}
	dns.set("127.0.0.1")
	p := pool.New(pool.WithSize(2))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)
	c, err := client.New(t.Context(), net.JoinHostPort("orders.test", server.port), client.WithPool(p), client.WithInsecure(),
		client.WithoutProxy(), client.WithDNSResolver(dns.option(t)))
	require.NoError(t, err)

	var conns []*grpc.ClientConn
	for range 2 {
		conn, err := c.GetConnection(t.Context())
		require.NoError(t, err)
		conns = append(conns, conn)
		require.NoError(t, check(t.Context(), conn, grpc.WaitForReady(true)))
	}
	require.NotSame(t, conns[0], conns[1])
	require.GreaterOrEqual(t, dns.calls.Load(), int32(2), "each pooled connection has a resolver")
	for _, conn := range conns {
		c.ReturnConnection(conn)
	}
}

// With WithBuiltinDNSResolver the connection is built with grpc-go's dns
// resolver, so the resolver options never take effect. The test does not
// wait for that resolver: it also looks up a TXT record, whose time depends
// on the machine's DNS.
func TestBuiltinDNSResolverOptOut(t *testing.T) {
	t.Parallel()
	server := startHealthBackend(t)
	target := net.JoinHostPort("orders.test", server.port)

	dns := &stubDNS{}
	dns.set("127.0.0.1")
	conn := connect(t, target, client.WithDNSResolver(dns.option(t)))
	require.NoError(t, check(t.Context(), conn, grpc.WaitForReady(true)))
	require.Positive(t, dns.calls.Load(), "the client resolves through its own resolver by default")

	builtin := &stubDNS{}
	builtin.set("127.0.0.1")
	conn = connect(t, target, client.WithDNSResolver(builtin.option(t)), client.WithBuiltinDNSResolver())
	// Leaving idle builds the resolver before the state changes.
	conn.Connect()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.True(t, conn.WaitForStateChange(ctx, connectivity.Idle))
	time.Sleep(100 * time.Millisecond)
	require.Zero(t, builtin.calls.Load())
}

func TestDNSResolverLeavesOtherSchemes(t *testing.T) {
	t.Parallel()
	server := startHealthBackend(t)
	static := manual.NewBuilderWithScheme("static")
	static.InitialState(resolver.State{Addresses: []resolver.Address{{Addr: server.addr}}})

	for _, tc := range []struct {
		name, target string
		opts         []client.Option
	}{
		{"passthrough", "passthrough:///" + server.addr, nil},
		{"custom scheme", "static:///orders", []client.Option{client.WithDialOptions(grpc.WithResolvers(static))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dns := &stubDNS{}
			conn := connect(t, tc.target, append(tc.opts, client.WithDNSResolver(dns.option(t)))...)
			require.NoError(t, check(t.Context(), conn, grpc.WaitForReady(true)))
			require.Zero(t, dns.calls.Load())
		})
	}
}

// A dns builder passed with WithDialOptions takes precedence over the
// client's resolver.
func TestDNSResolverYieldsToCallerBuilder(t *testing.T) {
	t.Parallel()
	server := startHealthBackend(t)
	own := manual.NewBuilderWithScheme("dns")
	own.InitialState(resolver.State{Addresses: []resolver.Address{{Addr: server.addr}}})
	dns := &stubDNS{}
	conn := connect(t, "orders.test:9000", client.WithDNSResolver(dns.option(t)), client.WithDialOptions(grpc.WithResolvers(own)))
	require.NoError(t, check(t.Context(), conn, grpc.WaitForReady(true)))
	require.Zero(t, dns.calls.Load())
}

// The resolver records its metrics on the client's meter.
func TestDNSResolverUsesClientMeter(t *testing.T) {
	t.Parallel()
	server := startHealthBackend(t)
	dns := &stubDNS{}
	dns.set("127.0.0.1")
	tc := metricstest.NewTestCollector()
	target := net.JoinHostPort("orders.test", server.port)
	conn := connect(t, target, client.WithMeter(tc), client.WithDNSResolver(dns.option(t)))
	require.NoError(t, check(t.Context(), conn, grpc.WaitForReady(true)))
	require.InDelta(t, 1, metricstest.GetGaugeValue(t, tc, "test_grpc_dns_resolver_addresses", "target", target), 0)
	require.GreaterOrEqual(t, metricstest.GetCounterValue(t, tc, "test_grpc_dns_resolver_lookups_total", "target", target, "result", "ok"), 1.0)
}
