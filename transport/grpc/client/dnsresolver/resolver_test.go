// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dnsresolver

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/resolver"
)

// The tests run in synctest bubbles: the fake clock makes refresh, backoff
// and timeout timing exact, and synctest.Test fails when a goroutine started
// in the bubble is still blocked after the test function returns, which
// proves that Close leaves no resolver goroutine behind. Tests that time
// refreshes exactly disable the refresh jitter.

const testTarget = "orders.test:9000"

var errTemporary = &net.DNSError{Err: "server misbehaving", Name: "orders.test", IsTemporary: true}

// fakeConn records what a resolver reports to its connection.
type fakeConn struct {
	resolver.ClientConn

	mu        sync.Mutex
	states    []resolver.State
	errs      []error
	updateErr error
}

func (c *fakeConn) UpdateState(s resolver.State) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states = append(c.states, s)
	return c.updateErr
}

func (c *fakeConn) ReportError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errs = append(c.errs, err)
}

func (c *fakeConn) setUpdateErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updateErr = err
}

// addresses returns the address list of every reported state.
func (c *fakeConn) addresses() [][]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]string, 0, len(c.states))
	for _, s := range c.states {
		addrs := make([]string, 0, len(s.Addresses))
		for i, a := range s.Addresses {
			addrs = append(addrs, a.Addr)
			if len(s.Endpoints[i].Addresses) != 1 || s.Endpoints[i].Addresses[0].Addr != a.Addr {
				panic("endpoint does not match address " + a.Addr)
			}
		}
		out = append(out, addrs)
	}
	return out
}

func (c *fakeConn) errors() []error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.errs)
}

// fakeDNS answers lookups from values the test changes and records the time
// of every lookup. With hang set, a lookup waits for its context.
type fakeDNS struct {
	mu    sync.Mutex
	addrs []string
	err   error
	hang  bool
	calls []time.Time
}

func (d *fakeDNS) lookup(ctx context.Context, _ string) ([]string, error) {
	d.mu.Lock()
	d.calls = append(d.calls, time.Now())
	addrs, err, hang := slices.Clone(d.addrs), d.err, d.hang
	d.mu.Unlock()
	if hang {
		<-ctx.Done()
		return nil, &net.DNSError{Err: ctx.Err().Error(), Name: "orders.test", IsTimeout: true}
	}
	return addrs, err
}

func (d *fakeDNS) answer(addrs []string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.addrs, d.err = addrs, err
}

func (d *fakeDNS) lookups() []time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.calls)
}

// gaps returns the pauses between consecutive lookups.
func (d *fakeDNS) gaps() []time.Duration {
	calls := d.lookups()
	gaps := make([]time.Duration, 0, len(calls))
	for i := 1; i < len(calls); i++ {
		gaps = append(gaps, calls[i].Sub(calls[i-1]))
	}
	return gaps
}

func notFound() error {
	return &net.DNSError{Err: "no such host", Name: "orders.test", IsNotFound: true}
}

func dnsTarget(authority, endpoint string) resolver.Target {
	return resolver.Target{URL: url.URL{Scheme: Scheme, Host: authority, Path: "/" + endpoint}}
}

// build starts a resolver for testTarget that looks up through dns.
func build(t *testing.T, dns *fakeDNS, opts ...Option) (*fakeConn, resolver.Resolver) {
	t.Helper()
	cc := &fakeConn{}
	r, err := NewBuilder(append([]Option{withLookup(dns.lookup)}, opts...)...).Build(dnsTarget("", testTarget), cc, resolver.BuildOptions{})
	require.NoError(t, err)
	return cc, r
}

func TestResolverRefreshDiscoversAddedAndRemovedAddresses(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"10.0.0.1"}}
		cc, r := build(t, dns, WithRefreshJitter(0))
		defer r.Close()
		synctest.Wait()
		require.Equal(t, [][]string{{"10.0.0.1:9000"}}, cc.addresses())

		// A scale-up is seen at the next refresh, with no ResolveNow.
		dns.answer([]string{"10.0.0.1", "10.0.0.2"}, nil)
		time.Sleep(DefaultRefreshInterval - time.Millisecond)
		synctest.Wait()
		require.Len(t, cc.addresses(), 1, "no lookup before the refresh interval")
		time.Sleep(time.Millisecond)
		synctest.Wait()
		require.Equal(t, []string{"10.0.0.1:9000", "10.0.0.2:9000"}, cc.addresses()[1])

		// So is a replaced address.
		dns.answer([]string{"10.0.0.2", "10.0.0.3"}, nil)
		time.Sleep(DefaultRefreshInterval)
		synctest.Wait()
		require.Equal(t, []string{"10.0.0.2:9000", "10.0.0.3:9000"}, cc.addresses()[2])
		require.Empty(t, cc.errors())
	})
}

func TestResolverSkipsUnchangedAddressSets(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"10.0.0.2", "10.0.0.1"}}
		cc, r := build(t, dns)
		defer r.Close()
		synctest.Wait()

		// Another order and a duplicate record are the same set.
		dns.answer([]string{"10.0.0.1", "10.0.0.2", "10.0.0.1"}, nil)
		time.Sleep(3 * DefaultRefreshInterval)
		synctest.Wait()
		require.Len(t, dns.lookups(), 4)
		require.Equal(t, [][]string{{"10.0.0.2:9000", "10.0.0.1:9000"}}, cc.addresses(), "the first answer keeps its DNS order")
	})
}

// A full outage: every server is gone, so DNS answers NXDOMAIN for longer
// than grpc-go's resolver takes to back off by tens of seconds. Lookups keep
// failing at most the backoff cap apart, and the addresses are reported no
// later than one cap after they return.
func TestResolverRecoversWithinBackoffCapAfterOutage(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{err: notFound()}
		cc, r := build(t, dns, WithRefreshJitter(0))
		defer r.Close()
		synctest.Wait()
		require.Len(t, cc.errors(), 1)
		require.ErrorIs(t, cc.errors()[0], errNoAddresses)
		require.ErrorContains(t, cc.errors()[0], "orders.test")

		dns.answer(nil, nil) // no records at all is the same outage
		time.Sleep(70 * time.Second)
		synctest.Wait()
		for _, gap := range dns.gaps() {
			require.LessOrEqual(t, gap, DefaultBackoffMaxDelay)
			require.GreaterOrEqual(t, gap, time.Duration(float64(DefaultBackoffBaseDelay)*(1-DefaultBackoffJitter)))
		}
		require.Empty(t, cc.addresses())
		require.Len(t, cc.errors(), len(dns.lookups()))

		dns.answer([]string{"10.0.0.7"}, nil)
		time.Sleep(DefaultBackoffMaxDelay)
		synctest.Wait()
		require.Equal(t, [][]string{{"10.0.0.7:9000"}}, cc.addresses())

		// Back to the refresh interval.
		time.Sleep(DefaultRefreshInterval)
		synctest.Wait()
		gaps := dns.gaps()
		require.Equal(t, DefaultRefreshInterval, gaps[len(gaps)-1])
	})
}

// A failed lookup keeps the connection's addresses; an answer that no server
// exists drops them once, so that servers returning on the same addresses get
// new subchannels.
func TestResolverDropsAddressesWhenDNSFindsNone(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"10.0.0.1"}}
		cc, r := build(t, dns, WithRefreshJitter(0), WithBackoffJitter(0))
		defer r.Close()
		synctest.Wait()
		require.Equal(t, [][]string{{"10.0.0.1:9000"}}, cc.addresses())

		// A failed lookup reports the error and keeps the addresses.
		dns.answer(nil, errTemporary)
		time.Sleep(DefaultRefreshInterval)
		synctest.Wait()
		require.Len(t, cc.errors(), 1)
		require.Len(t, cc.addresses(), 1)

		// NXDOMAIN drops them with an empty state, once.
		dns.answer(nil, notFound())
		time.Sleep(DefaultBackoffBaseDelay)
		synctest.Wait()
		require.Equal(t, [][]string{{"10.0.0.1:9000"}, {}}, cc.addresses())
		require.Len(t, cc.errors(), 2)
		require.ErrorIs(t, cc.errors()[1], errNoAddresses)
		time.Sleep(DefaultBackoffMaxDelay)
		synctest.Wait()
		require.Len(t, cc.addresses(), 2, "further empty answers report only the error")
		require.Len(t, cc.errors(), len(dns.lookups())-1)

		// The same address comes back and is reported again.
		dns.answer([]string{"10.0.0.1"}, nil)
		time.Sleep(DefaultBackoffMaxDelay)
		synctest.Wait()
		require.Equal(t, []string{"10.0.0.1:9000"}, cc.addresses()[2])
	})
}

func TestResolverBackoffGrowsToItsCap(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{err: errTemporary}
		_, r := build(t, dns, WithBackoffJitter(0))
		defer r.Close()
		time.Sleep(30 * time.Second)
		synctest.Wait()

		want := []time.Duration{time.Second, 1600 * time.Millisecond, 2560 * time.Millisecond, 4096 * time.Millisecond, 5 * time.Second, 5 * time.Second}
		gaps := dns.gaps()
		require.GreaterOrEqual(t, len(gaps), len(want))
		for i, w := range want {
			require.InDelta(t, float64(w), float64(gaps[i]), float64(time.Microsecond), "gap %d", i)
		}
	})
}

// At the cap the jitter only shortens the delay: the delays stay spread
// instead of half of them being exactly the cap.
func TestResolverBackoffJitterStaysBelowTheCap(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{err: errTemporary}
		_, r := build(t, dns)
		defer r.Close()
		time.Sleep(10 * time.Minute)
		synctest.Wait()

		// From the sixth failure on the delay is at the cap.
		atCap := dns.gaps()[5:]
		require.Greater(t, len(atCap), 100)
		low := time.Duration(float64(DefaultBackoffMaxDelay) * (1 - DefaultBackoffJitter))
		exact := 0
		for _, gap := range atCap {
			require.GreaterOrEqual(t, gap, low)
			require.LessOrEqual(t, gap, DefaultBackoffMaxDelay)
			if gap == DefaultBackoffMaxDelay {
				exact++
			}
		}
		require.LessOrEqual(t, exact, 1, "delays at the cap are not clipped to it")
	})
}

// Refreshes come at most the refresh interval apart, shortened at random by
// up to the refresh jitter, which is bounded by MaxRefreshJitter.
func TestResolverRefreshJitter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		opts   []Option
		jitter float64
	}{
		{"default", nil, DefaultRefreshJitter},
		{"above the maximum", []Option{WithRefreshJitter(0.9)}, MaxRefreshJitter},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				dns := &fakeDNS{addrs: []string{"10.0.0.1"}}
				_, r := build(t, dns, tc.opts...)
				defer r.Close()
				time.Sleep(50 * DefaultRefreshInterval)
				synctest.Wait()

				gaps := dns.gaps()
				require.GreaterOrEqual(t, len(gaps), 50)
				low := time.Duration(float64(DefaultRefreshInterval) * (1 - tc.jitter))
				for _, gap := range gaps {
					require.GreaterOrEqual(t, gap, low)
					require.LessOrEqual(t, gap, DefaultRefreshInterval)
				}
				require.Greater(t, len(slices.Compact(slices.Sorted(slices.Values(gaps)))), 1, "the gaps vary")
			})
		})
	}
}

func TestResolverResolveNowShortensBackoff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{err: errTemporary}
		_, r := build(t, dns, WithBackoffBaseDelay(10*time.Second), WithBackoffMaxDelay(10*time.Second), WithBackoffJitter(0))
		defer r.Close()
		synctest.Wait()
		start := dns.lookups()[0]

		// Too early: the lookup waits for the minimum interval, not for the
		// 10s backoff.
		time.Sleep(500 * time.Millisecond)
		r.ResolveNow(resolver.ResolveNowOptions{})
		synctest.Wait()
		require.Len(t, dns.lookups(), 1)
		time.Sleep(DefaultMinInterval - 500*time.Millisecond)
		synctest.Wait()
		require.Equal(t, []time.Time{start, start.Add(DefaultMinInterval)}, dns.lookups())

		// After the minimum interval a request starts a lookup at once.
		time.Sleep(5 * time.Second)
		r.ResolveNow(resolver.ResolveNowOptions{})
		synctest.Wait()
		require.Len(t, dns.lookups(), 3)
		require.Equal(t, start.Add(DefaultMinInterval+5*time.Second), dns.lookups()[2])

		// Without a request, the backoff applies.
		time.Sleep(10*time.Second - time.Millisecond)
		synctest.Wait()
		require.Len(t, dns.lookups(), 3)
		time.Sleep(time.Millisecond)
		synctest.Wait()
		require.Len(t, dns.lookups(), 4)
	})
}

func TestResolverResolveNowIsRateLimited(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"10.0.0.1"}}
		cc, r := build(t, dns)
		defer r.Close()
		synctest.Wait()

		// A burst of requests, as when several subchannels fail, costs one
		// lookup per minimum interval.
		for range 10 {
			r.ResolveNow(resolver.ResolveNowOptions{})
		}
		synctest.Wait()
		require.Len(t, dns.lookups(), 1)
		for range 3 {
			r.ResolveNow(resolver.ResolveNowOptions{})
			time.Sleep(DefaultMinInterval)
			synctest.Wait()
		}
		require.Equal(t, []time.Duration{DefaultMinInterval, DefaultMinInterval, DefaultMinInterval}, dns.gaps())
		require.Len(t, cc.addresses(), 1, "the unchanged set is reported once")
	})
}

func TestResolverMinIntervalIsCappedByRefreshInterval(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"10.0.0.1"}}
		_, r := build(t, dns, WithRefreshInterval(time.Second), WithRefreshJitter(0), WithMinInterval(time.Minute))
		defer r.Close()
		synctest.Wait()
		r.ResolveNow(resolver.ResolveNowOptions{})
		time.Sleep(time.Second)
		synctest.Wait()
		require.Equal(t, []time.Duration{time.Second}, dns.gaps())
	})
}

func TestResolverLookupTimeout(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{hang: true}
		cc, r := build(t, dns, WithLookupTimeout(3*time.Second))
		defer r.Close()
		time.Sleep(3*time.Second - time.Millisecond)
		synctest.Wait()
		require.Empty(t, cc.errors())

		time.Sleep(time.Millisecond)
		synctest.Wait()
		require.Len(t, cc.errors(), 1)
		require.ErrorContains(t, cc.errors()[0], "lookup orders.test")
		require.NotErrorIs(t, cc.errors()[0], errNoAddresses)
	})
}

func TestResolverCloseStopsLookupAndTimers(t *testing.T) {
	t.Parallel()

	t.Run("lookup in flight", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			dns := &fakeDNS{hang: true}
			cc, r := build(t, dns)
			synctest.Wait()
			r.Close()
			r.Close()
			r.ResolveNow(resolver.ResolveNowOptions{})
			time.Sleep(time.Hour)
			synctest.Wait()
			require.Len(t, dns.lookups(), 1)
			require.Empty(t, cc.errors(), "a canceled lookup is not reported")
			require.Empty(t, cc.addresses())
		})
	})

	for _, tc := range []struct {
		name string
		dns  *fakeDNS
	}{
		{"waiting to refresh", &fakeDNS{addrs: []string{"10.0.0.1"}}},
		{"backing off", &fakeDNS{err: errTemporary}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				_, r := build(t, tc.dns)
				synctest.Wait()
				r.Close()
				r.ResolveNow(resolver.ResolveNowOptions{})
				time.Sleep(time.Hour)
				synctest.Wait()
				require.Len(t, tc.dns.lookups(), 1)
			})
		})
	}
}

// An address set the connection rejects is retried with backoff and sent
// again, as for a failed lookup.
func TestResolverRetriesRejectedState(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"10.0.0.1"}}
		cc := &fakeConn{updateErr: balancer.ErrBadResolverState}
		r, err := NewBuilder(withLookup(dns.lookup), WithBackoffJitter(0)).Build(dnsTarget("", testTarget), cc, resolver.BuildOptions{})
		require.NoError(t, err)
		defer r.Close()
		synctest.Wait()
		time.Sleep(DefaultBackoffBaseDelay)
		synctest.Wait()
		require.Len(t, cc.addresses(), 2)

		cc.setUpdateErr(nil)
		time.Sleep(time.Duration(float64(DefaultBackoffBaseDelay) * DefaultBackoffMultiplier))
		synctest.Wait()
		require.Len(t, cc.addresses(), 3)
		time.Sleep(DefaultRefreshInterval)
		synctest.Wait()
		require.Len(t, cc.addresses(), 3, "an accepted unchanged set is not sent again")
		require.Empty(t, cc.errors())
	})
}

func TestResolverLogsOnlyTransitions(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		dns := &fakeDNS{addrs: []string{"10.0.0.1"}}
		_, r := build(t, dns, WithLogger(logger))
		defer r.Close()

		time.Sleep(2 * DefaultRefreshInterval)
		synctest.Wait()
		require.Empty(t, buf.String(), "healthy polls are not logged")

		dns.answer(nil, notFound())
		time.Sleep(DefaultRefreshInterval + 20*time.Second)
		synctest.Wait()
		dns.answer([]string{"10.0.0.2"}, nil)
		time.Sleep(DefaultBackoffMaxDelay + 2*DefaultRefreshInterval)
		synctest.Wait()

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		require.Len(t, lines, 2, buf.String())
		require.Contains(t, lines[0], "level=WARN")
		require.Contains(t, lines[0], `msg="dns resolution failing"`)
		require.Contains(t, lines[0], "target=orders.test:9000")
		require.Contains(t, lines[0], "no addresses for host")
		require.Contains(t, lines[1], "level=INFO")
		require.Contains(t, lines[1], `msg="dns resolution recovered"`)
		require.Contains(t, lines[1], "addresses=1")
		require.Contains(t, lines[1], "failed_attempts=")
	})
}

func TestResolverMetrics(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"10.0.0.1", "10.0.0.2"}}
		_, r := build(t, dns, WithMeter(tc), WithRefreshJitter(0), WithBackoffJitter(0))
		defer r.Close()
		synctest.Wait()

		lookups := func(result string) float64 {
			return metricstest.GetCounterValue(t, tc, "test_grpc_dns_resolver_lookups_total", "target", testTarget, "result", result)
		}
		addresses := func() float64 {
			return metricstest.GetGaugeValue(t, tc, "test_grpc_dns_resolver_addresses", "target", testTarget)
		}
		require.InDelta(t, 1, lookups("ok"), 0)
		require.InDelta(t, 2, addresses(), 0)

		// A failed lookup keeps the last conclusive count.
		dns.answer(nil, errTemporary)
		time.Sleep(DefaultRefreshInterval)
		synctest.Wait()
		require.InDelta(t, 1, lookups("error"), 0)
		require.InDelta(t, 2, addresses(), 0)

		dns.answer(nil, notFound())
		time.Sleep(DefaultBackoffBaseDelay)
		synctest.Wait()
		require.InDelta(t, 1, lookups("empty"), 0)
		require.InDelta(t, 0, addresses(), 0)
	})
}

func TestResolverRejectsNonIPAnswers(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"10.0.0.1", "orders.internal"}}
		cc, r := build(t, dns)
		defer r.Close()
		synctest.Wait()
		require.Empty(t, cc.addresses())
		require.Len(t, cc.errors(), 1)
		require.ErrorContains(t, cc.errors()[0], `"orders.internal"`)
	})
}

func TestResolverFormatsIPv6Answers(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"2001:db8::1", "fe80::1%eth0", "10.0.0.1"}}
		cc, r := build(t, dns)
		defer r.Close()
		synctest.Wait()
		require.Equal(t, [][]string{{"[2001:db8::1]:9000", "[fe80::1%eth0]:9000", "10.0.0.1:9000"}}, cc.addresses())
	})
}

func TestLookupErrorsWrapTheCause(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cause := errors.New("refused")
		dns := &fakeDNS{err: cause}
		cc, r := build(t, dns)
		defer r.Close()
		synctest.Wait()
		require.ErrorIs(t, cc.errors()[0], cause)
	})
}
