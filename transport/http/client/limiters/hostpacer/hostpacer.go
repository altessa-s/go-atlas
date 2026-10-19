// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hostpacer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/transport/http/client/limiters"

	"golang.org/x/time/rate"
)

var _ limiters.RequestsLimiter = (*Pacer)(nil)

// ErrInvalidRate is returned by [New] for a non-positive interval or burst.
var ErrInvalidRate = errors.New("hostpacer: interval and burst must be positive")

// Pacer spaces requests per key (a host): each key gets its own token bucket
// refilled once per interval up to burst tokens. Unlike a rejecting limiter,
// [Pacer.Allow] waits for the key's next slot, so a fan-out over one host is
// throttled rather than failed. It is safe for concurrent use.
//
// Buckets are created on first use. When more than the configured maximum of
// keys are tracked, a new key first drops the buckets that have refilled
// completely; that is lossless, as a new bucket starts full. Buckets still
// refilling are never dropped, so memory is bounded by the keys used within
// one burst × interval window.
type Pacer struct {
	every    rate.Limit
	burst    int
	maxHosts int
	now      func() time.Time

	mu    sync.Mutex
	hosts map[string]*rate.Limiter
}

// New returns a Pacer allowing one request per interval per key with the
// given burst. A non-positive interval or burst returns [ErrInvalidRate].
func New(interval time.Duration, burst int, opts ...Option) (*Pacer, error) {
	if interval <= 0 || burst <= 0 {
		return nil, fmt.Errorf("%w: interval %s, burst %d", ErrInvalidRate, interval, burst)
	}
	o := newOptions(opts...)
	return &Pacer{
		every:    rate.Every(interval),
		burst:    burst,
		maxHosts: o.maxHosts,
		now:      o.now,
		hosts:    make(map[string]*rate.Limiter),
	}, nil
}

// Allow waits until key may send its next request. The slot is reserved
// under the pacer's lock together with the bucket lookup, so a bucket is
// never dropped between the lookup and the reservation. It returns the
// context's error when the wait is cut short (the slot is released), and an
// error wrapping [context.DeadlineExceeded] without waiting when the slot
// lies beyond the context deadline.
func (p *Pacer) Allow(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r, now := p.reserve(key)
	delay := r.DelayFrom(now)
	if delay <= 0 {
		return nil
	}
	if deadline, ok := ctx.Deadline(); ok && deadline.Sub(now) < delay {
		r.CancelAt(now)
		return fmt.Errorf("%w: next slot for %q in %s", context.DeadlineExceeded, key, delay)
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		r.CancelAt(p.now())
		return ctx.Err()
	}
}

// Len returns the number of keys currently tracked.
func (p *Pacer) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.hosts)
}

// reserve takes the next slot of key's bucket, creating the bucket on first
// use (after a sweep when the pacer tracks more than maxHosts keys). The
// lookup and the reservation happen under mu, so the sweep, which drops only
// full buckets, never drops a bucket with a reservation.
func (p *Pacer) reserve(key string) (*rate.Reservation, time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	lim, ok := p.hosts[key]
	if !ok {
		if p.maxHosts > 0 && len(p.hosts) >= p.maxHosts {
			p.sweep(now)
		}
		lim = rate.NewLimiter(p.every, p.burst)
		p.hosts[key] = lim
	}
	return lim.ReserveN(now, 1), now
}

// sweep drops the buckets that are full at now. Callers hold mu.
func (p *Pacer) sweep(now time.Time) {
	full := float64(p.burst)
	for key, lim := range p.hosts {
		if lim.TokensAt(now) >= full {
			delete(p.hosts, key)
		}
	}
}
