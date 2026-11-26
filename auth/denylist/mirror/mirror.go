// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

// Source streams the keys an authoritative revocation store currently considers
// revoked. It is the consumer-side view [Cache] reads on every [Cache.Refresh];
// [github.com/altessa-s/go-atlas/auth/denylist/storages/redis.Store.RawSource] supplies bare Redis keys.
type Source interface {
	// StreamValues yields every currently-revoked key. A non-nil error ends the
	// stream and aborts the refresh, leaving the previous snapshot in place.
	StreamValues(ctx context.Context) iter.Seq2[string, error]
}

// snapshot is one immutable published state: the revoked set, whether it came
// from a successful refresh, and when. Readers load it once, so a set is never
// paired with another refresh's timestamp.
type snapshot struct {
	set         map[string]struct{}
	ready       bool
	refreshedAt time.Time
}

// Serving states, used to log only when the state changes.
const (
	stateServing int32 = iota
	stateUnprimed
	stateStale
)

// Cache is a synchronous, periodically-refreshed local snapshot of a
// distributed denylist. It satisfies [github.com/altessa-s/go-atlas/auth/jwt.RevocationChecker], so a jwt or
// selfjwt verifier can enforce distributed revocation without a network call on
// the hot path. Reads are lock-free; a nil *Cache is not valid — construct with
// [New].
//
// A Cache fails closed: until [Cache.Prime] (or the first [Cache.Refresh])
// succeeds, and whenever the snapshot is older than [WithMaxStaleness],
// [Cache.IsRevoked] returns [jwt.ErrRevocationUnavailable] instead of "not
// revoked".
type Cache struct {
	refresh chan struct{} // serializes source reads and snapshot publication
	src     Source
	metrics *Metrics
	logger  *slog.Logger
	now     func() time.Time
	opts    *options
	snap    atomic.Pointer[snapshot]
	state   atomic.Int32
}

// New returns a Cache backed by src. The cache starts unprimed and refuses to
// answer — [Cache.IsRevoked] returns [jwt.ErrRevocationUnavailable] — so call
// [Cache.Prime] before serving traffic and then [Cache.Refresh] on a schedule
// to keep it current. src must be non-nil. Pass [WithMetrics] to record refresh
// outcomes and snapshot size, [WithMaxStaleness] to bound the snapshot's age
// and [WithInsecureServeUnprimed] to opt out of the unprimed refusal.
func New(src Source, opts ...Option) *Cache {
	o := newOptions(opts...)
	c := &Cache{src: src, metrics: o.metrics, refresh: make(chan struct{}, 1), opts: o, now: o.clock, logger: o.logger}
	if c.logger == nil {
		c.logger = slog.New(slog.DiscardHandler)
	}
	if c.now == nil {
		c.now = time.Now
	}
	c.snap.Store(&snapshot{set: map[string]struct{}{}})
	return c
}

// IsRevoked reports whether id (a jti or subject) is in the current snapshot.
// It reads the snapshot lock-free and never blocks. It returns
// [jwt.ErrRevocationUnavailable] when the cache is unprimed (unless
// [WithInsecureServeUnprimed]) or older than [WithMaxStaleness].
func (c *Cache) IsRevoked(id string) (bool, error) {
	s := c.snap.Load()
	if !s.ready {
		if !c.opts.serveUnprimed {
			c.unavailable(stateUnprimed, reasonUnprimed)
			return false, fmt.Errorf("%w: denylist mirror not primed", jwt.ErrRevocationUnavailable)
		}
	} else if c.tooOld(s) {
		c.unavailable(stateStale, reasonStale)
		return false, fmt.Errorf("%w: denylist mirror snapshot older than %s", jwt.ErrRevocationUnavailable, c.opts.maxStaleness)
	}
	// Load first: the swap writes a shared cache line, and the state rarely
	// changes while lookups run on every verification.
	if c.state.Load() != stateServing && c.state.Swap(stateServing) != stateServing {
		c.logger.Info("denylist mirror: serving revocation checks again")
	}
	_, ok := s.set[id]
	return ok, nil
}

func (c *Cache) tooOld(s *snapshot) bool {
	return c.opts.maxStaleness > 0 && c.now().Sub(s.refreshedAt) > c.opts.maxStaleness
}

// unavailable counts a refused lookup and logs once per change of state.
func (c *Cache) unavailable(state int32, reason string) {
	c.metrics.recordUnavailable(reason)
	if c.state.Swap(state) != state {
		c.logger.Error("denylist mirror: refusing revocation checks, tokens with a jti are rejected", slog.String("reason", reason))
	}
}

// Prime performs the first synchronous refresh and returns the source error, so
// a service can refuse to start (or report not-ready) while the denylist is
// unreachable. It is [Cache.Refresh] under the name that states its role.
func (c *Cache) Prime(ctx context.Context) error {
	return c.Refresh(ctx)
}

// Ready reports whether the cache has a snapshot it will answer from: primed
// and, when [WithMaxStaleness] is set, not older than it.
func (c *Cache) Ready() bool {
	s := c.snap.Load()
	return s.ready && !c.tooOld(s)
}

// Age returns the time since the last successful refresh, or zero for a cache
// that has never refreshed.
func (c *Cache) Age() time.Duration {
	s := c.snap.Load()
	if !s.ready {
		return 0
	}
	return c.now().Sub(s.refreshedAt)
}

// Refresh rebuilds the snapshot from the authoritative store and atomically
// swaps it in. On a stream error the previous snapshot is kept and the error is
// returned: a transient store failure degrades to slightly staler data rather
// than an empty denylist, until [WithMaxStaleness] makes the cache refuse to
// answer. Refresh is intended to run on a scheduler; it is safe to call
// concurrently with [Cache.IsRevoked].
func (c *Cache) Refresh(ctx context.Context) error {
	select {
	case c.refresh <- struct{}{}:
		defer func() { <-c.refresh }()
	case <-ctx.Done():
		c.metrics.recordRefresh(resultError, 0)
		return ctx.Err()
	}
	// The snapshot is as old as the start of the scan: a key revoked while it
	// ran may be missing from it.
	startedAt := c.now()
	next := make(map[string]struct{})
	for key, err := range c.src.StreamValues(ctx) {
		if err != nil {
			c.metrics.recordRefresh(resultError, 0)
			return &refreshError{cause: err}
		}
		next[key] = struct{}{}
	}
	// A source whose iterator stops silently on cancellation would otherwise
	// publish a partial set as a ready snapshot.
	if err := ctx.Err(); err != nil {
		c.metrics.recordRefresh(resultError, 0)
		return err
	}
	c.snap.Store(&snapshot{set: next, ready: true, refreshedAt: startedAt})
	c.metrics.recordRefresh(resultOK, len(next))
	return nil
}

// refreshError reports a failed source scan by the type of the source error
// only: the text of a dependency error may carry upstream detail, such as a
// DSN or a response body. errors.Is and errors.As still reach the cause.
type refreshError struct{ cause error }

func (e *refreshError) Error() string {
	return fmt.Sprintf("denylist mirror: refresh: source error (%T)", e.cause)
}

func (e *refreshError) Unwrap() error { return e.cause }

// Len reports the number of keys in the current snapshot, for observability and
// tests.
func (c *Cache) Len() int {
	return len(c.snap.Load().set)
}
