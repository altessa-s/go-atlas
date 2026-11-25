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

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/transport/grpc/client/internal/dnshook"

	"google.golang.org/grpc/resolver"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
)

var _ resolver.Resolver = (*dnsResolver)(nil)

// errNoAddresses reports a lookup that found no address for the host.
var errNoAddresses = errors.New("no addresses for host")

// dnsResolver resolves the host of one connection's target. A single
// goroutine (run) coordinates the lookups and reports to the
// connection; ResolveNow and Close only signal it.
type dnsResolver struct {
	cc      resolver.ClientConn
	lookup  dnshook.LookupFunc
	opts    *options
	metrics *builderMetrics
	host    string
	port    string
	// target is the host:port log attribute.
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
// reported as an error and retried with backoff. A conclusive negative for
// both families submits an empty update. Transitions are logged.
func (r *dnsResolver) run() {
	defer close(r.done)
	var (
		state        resolutionState
		failures     int
		failingSince time.Time
	)
	defer func() { r.metrics.addresses.Sub(float64(len(state.sorted))) }()
	for {
		// The next lookup satisfies any request queued before it starts,
		// including one that raced with the timer in sleep.
		select {
		case <-r.resolveNow:
		default:
		}
		next, lookupErr := r.resolve(state.families)
		if r.ctx.Err() != nil {
			return
		}
		err := r.publish(&state, next, lookupErr, failures > 0)

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
			r.opts.logger.InfoContext(r.ctx, "dns resolution recovered", "target", r.target, "addresses", len(state.sorted),
				"failed_attempts", failures, "downtime", time.Since(failingSince))
			failures = 0
		}
		if !r.sleep(wait) {
			return
		}
	}
}

// resolutionState tracks the most recently submitted addresses.
type resolutionState struct {
	families addressFamilies
	sorted   []string
	rejected bool
}

// publish applies a lookup result and returns any error that calls for a retry.
func (r *dnsResolver) publish(state *resolutionState, next addressFamilies, lookupErr error, recovering bool) error {
	switch {
	case errors.Is(lookupErr, errNoAddresses):
		r.metrics.empty.Inc()
		if len(state.sorted) != 0 {
			// Round-robin removes its old subchannels on an empty update.
			// Other policies can reject the update and keep them.
			_ = r.cc.UpdateState(resolver.State{})
			r.metrics.addresses.Sub(float64(len(state.sorted)))
		}
		*state = resolutionState{}
	case lookupErr != nil:
		r.metrics.failed.Inc()
	default:
		r.metrics.ok.Inc()
	}

	var updateErr error
	if addrs := slices.Concat(next[:]...); len(addrs) != 0 {
		sorted := slices.Sorted(slices.Values(addrs))
		if state.rejected || (lookupErr == nil && recovering) || !slices.Equal(sorted, state.sorted) {
			updateErr = r.cc.UpdateState(newState(addrs))
			r.metrics.addresses.Add(float64(len(sorted) - len(state.sorted)))
		}
		*state = resolutionState{families: next, sorted: sorted, rejected: updateErr != nil}
	}
	if lookupErr != nil {
		r.cc.ReportError(lookupErr)
		return lookupErr
	}
	return updateErr
}

// addressFamilies stores the IPv4 and IPv6 addresses separately.
type addressFamilies [2][]string

// resolve queries A and AAAA independently within a shared timeout. Each
// conclusive answer replaces that family's addresses, while a failed query
// keeps the family's previous addresses.
func (r *dnsResolver) resolve(previous addressFamilies) (addressFamilies, error) {
	ctx, cancel := context.WithTimeout(r.ctx, r.opts.lookupTimeout)
	defer cancel()

	networks := [...]string{"ip4", "ip6"}
	next := previous
	var errs [len(networks)]error
	// Schedule both queries under the resolver lifetime and pass the shared
	// deadline to each query, so a timeout cannot skip the second outcome.
	if err := concurrency.Process(r.ctx, []int{0, 1}, func(_ context.Context, i int) error {
		addrs, err := r.lookupFamily(ctx, networks[i])
		if err == nil {
			next[i] = addrs
		}
		errs[i] = err
		return nil
	}, concurrency.WithConcurrency[int](len(networks))); err != nil {
		return previous, err
	}
	if err := errors.Join(errs[:]...); err != nil {
		return next, err
	}
	if len(next[0])+len(next[1]) == 0 {
		return next, fmt.Errorf("dnsresolver: %w %s", errNoAddresses, r.host)
	}
	return next, nil
}

func (r *dnsResolver) lookupFamily(ctx context.Context, network string) ([]string, error) {
	hosts, err := r.lookup(ctx, network, r.host)
	if dnsErr, ok := errors.AsType[*net.DNSError](err); ok && dnsErr.IsNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dnsresolver: lookup %s (%s): %w", r.host, network, err)
	}
	addrs := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if !isIP(host) {
			return nil, fmt.Errorf("dnsresolver: lookup %s (%s) returned a non-IP address", r.host, network)
		}
		if addr := net.JoinHostPort(host, r.port); !slices.Contains(addrs, addr) {
			addrs = append(addrs, addr)
		}
	}
	return addrs, nil
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
