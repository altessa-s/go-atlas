// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dnsresolver

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net"
	"slices"
	"time"

	"github.com/altessa-s/go-atlas/transport/grpc/client/internal/dnshook"

	"google.golang.org/grpc/resolver"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
)

var _ resolver.Resolver = (*dnsResolver)(nil)

// errNoAddresses reports a lookup that found no address for the host.
var errNoAddresses = errors.New("no addresses for host")

// dnsResolver resolves the host of one connection's target. A single
// goroutine (run) performs the lookups one at a time and reports to the
// connection; ResolveNow and Close only signal it.
type dnsResolver struct {
	cc      resolver.ClientConn
	lookup  dnshook.LookupFunc
	opts    *options
	metrics targetMetrics
	host    string
	port    string
	// target is host:port, the metric label and log attribute.
	target string

	// ctx bounds every lookup and wait; Close cancels it.
	ctx    context.Context
	cancel context.CancelFunc
	// resolveNow holds at most one pending ResolveNow request.
	resolveNow chan struct{}
	// done is closed when run returns.
	done chan struct{}
}

// ResolveNow asks for a lookup before the next scheduled one. The lookup
// starts no earlier than the minimum interval after the previous one ended.
// It never blocks; requests that arrive while one is pending are merged.
func (r *dnsResolver) ResolveNow(resolver.ResolveNowOptions) {
	select {
	case r.resolveNow <- struct{}{}:
	default:
	}
}

// Close cancels the lookup in flight and the pending wait, and returns once
// the resolver goroutine has exited. It is idempotent.
func (r *dnsResolver) Close() {
	r.cancel()
	<-r.done
}

// run resolves the host until Close. A successful lookup is reported when
// the address set changed or the previous attempt failed, and the next one
// follows after the jittered refresh interval. A failed or empty lookup is
// reported as an error and retried with backoff; an empty one also drops the
// connection's addresses. Transitions between the two are logged.
func (r *dnsResolver) run() {
	defer close(r.done)
	var (
		current      []string // sorted addresses of the last accepted state
		holding      bool     // the connection was given addresses since the last drop
		failures     int      // consecutive failed attempts
		failingSince time.Time
	)
	for {
		addrs, result, err := r.resolve()
		if r.ctx.Err() != nil {
			return
		}
		r.metrics.record(result, len(addrs))
		if err != nil {
			if result == resultEmpty && holding {
				// DNS answered that no server exists, as for a headless service
				// without ready pods. Dropping the addresses closes their
				// subchannels, so a server that returns, even on its old address
				// (a container restarted in place keeps its pod IP), gets a new
				// subchannel instead of one waiting out its connection backoff.
				// A balancer rejects the empty state, which changes nothing here.
				_ = r.cc.UpdateState(resolver.State{})
				current, holding = nil, false
			}
			// Without a drop the connection keeps its addresses. Before the
			// first state the error is what RPCs fail with.
			r.cc.ReportError(err)
		} else if sorted := slices.Sorted(slices.Values(addrs)); failures > 0 || !slices.Equal(sorted, current) {
			// The connection takes the state even when it returns an error;
			// the error asks for a new attempt with backoff.
			holding = true
			if err = r.cc.UpdateState(newState(addrs)); err == nil {
				current = sorted
			}
		}

		wait := r.refreshDelay()
		switch {
		case err != nil:
			if failures == 0 {
				failingSince = time.Now()
				r.opts.logger.WarnContext(r.ctx, "dns resolution failing", "target", r.target, "error", err)
			}
			failures++
			wait = r.backoff(failures)
		case failures > 0:
			r.opts.logger.InfoContext(r.ctx, "dns resolution recovered", "target", r.target, "addresses", len(addrs),
				"failed_attempts", failures, "downtime", time.Since(failingSince))
			failures = 0
		}
		if !r.sleep(wait) {
			return
		}
	}
}

// resolve looks up the host once, bounded by the lookup timeout, and returns
// its addresses with the target port in the order DNS gave them, without
// duplicates. No address is an error.
func (r *dnsResolver) resolve() ([]string, lookupResult, error) {
	ctx, cancel := context.WithTimeout(r.ctx, r.opts.lookupTimeout)
	defer cancel()
	hosts, err := r.lookup(ctx, r.host)
	if dnsErr, ok := errors.AsType[*net.DNSError](err); ok && dnsErr.IsNotFound {
		return nil, resultEmpty, fmt.Errorf("dnsresolver: %w %s", errNoAddresses, r.host)
	}
	if err != nil {
		return nil, resultError, fmt.Errorf("dnsresolver: lookup %s: %w", r.host, err)
	}
	addrs := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if !isIP(host) {
			return nil, resultError, fmt.Errorf("dnsresolver: lookup %s returned %q, which is not an IP address", r.host, host)
		}
		if addr := net.JoinHostPort(host, r.port); !slices.Contains(addrs, addr) {
			addrs = append(addrs, addr)
		}
	}
	if len(addrs) == 0 {
		return nil, resultEmpty, fmt.Errorf("dnsresolver: %w %s", errNoAddresses, r.host)
	}
	return addrs, resultOK, nil
}

// backoff returns the delay before the next attempt after failures
// consecutive failed ones: the base delay grown by the multiplier per further
// failure, randomized by the jitter fraction and never above the maximum.
// Near the maximum the jitter range is cut at it rather than the delay
// clipped to it: clipping would give half of the retries at the cap the same
// delay and keep clients that reached it together in lockstep.
func (r *dnsResolver) backoff(failures int) time.Duration {
	o := r.opts
	ceiling := float64(o.backoffMaxDelay)
	delay := min(float64(o.backoffBaseDelay)*math.Pow(o.backoffMultiplier, float64(failures-1)), ceiling)
	low, high := delay*(1-o.backoffJitter), min(delay*(1+o.backoffJitter), ceiling)
	return time.Duration(low + (high-low)*rand.Float64()) // #nosec G404 -- jitter, not a secret
}

// refreshDelay returns the pause before the next refresh: the refresh
// interval shortened by a random fraction up to the refresh jitter. Clients
// started together, by one rollout for example, so drift apart instead of
// querying the DNS server in lockstep, and a change is still seen within the
// refresh interval.
func (r *dnsResolver) refreshDelay() time.Duration {
	return time.Duration(float64(r.opts.refreshInterval) * (1 - r.opts.refreshJitter*rand.Float64())) // #nosec G404 -- jitter, not a secret
}

// sleep waits until the next lookup is due, wait from now. A ResolveNow
// request brings it forward, though not before the minimum interval from
// now. It reports false when the resolver is closed.
func (r *dnsResolver) sleep(wait time.Duration) bool {
	start := time.Now()
	due := start.Add(wait)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return false
		case <-timer.C:
			return true
		case <-r.resolveNow:
			if earliest := start.Add(r.opts.minInterval); earliest.Before(due) {
				due = earliest
				timer.Reset(time.Until(due))
			}
		}
	}
}

// newState returns the resolver state of addrs, one endpoint per address.
func newState(addrs []string) resolver.State {
	addresses := slices.Collect(coreslices.Map(addrs, func(addr string) resolver.Address { return resolver.Address{Addr: addr} }))
	return resolver.State{
		Addresses: addresses,
		Endpoints: slices.Collect(coreslices.Map(addresses, func(addr resolver.Address) resolver.Endpoint {
			return resolver.Endpoint{Addresses: []resolver.Address{addr}}
		})),
	}
}
