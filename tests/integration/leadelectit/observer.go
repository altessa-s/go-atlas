// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelectit

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Elector is the slice of the leadelect API the Observer needs. Declared here,
// on the consumer side, so the checker can also be pointed at a fake.
type Elector interface {
	NodeId() string
	IsLeader() bool
	Fence() uint64
}

// Claim is one node's answer in one polling round.
type Claim struct {
	Node  string
	Fence uint64
}

// Round is a single sweep across every elector, recording those that claimed
// leadership. Rounds with no claim are kept: a gap is how a failover looks, and
// its length is what the liveness assertions measure.
type Round struct {
	At     time.Time
	Claims []Claim
}

// Term is a maximal run of consecutive rounds claimed by the same node.
type Term struct {
	Node       string
	FirstFence uint64
	LastFence  uint64
	Rounds     int
}

// Observer polls a set of electors and records what they claimed.
//
// It is safe to read the recording only after Stop returns.
type Observer struct {
	electors []Elector
	interval time.Duration

	mu     sync.Mutex
	rounds []Round

	cancel context.CancelFunc
	done   chan struct{}
}

// NewObserver returns an Observer over electors, polling every interval.
func NewObserver(interval time.Duration, electors ...Elector) *Observer {
	return &Observer{electors: electors, interval: interval}
}

// Start begins polling. It samples once immediately so a scenario that starts
// and stops the Observer inside one lease still records something.
func (o *Observer) Start(ctx context.Context) {
	ctx, o.cancel = context.WithCancel(ctx)
	o.done = make(chan struct{})

	go func() {
		defer close(o.done)

		ticker := time.NewTicker(o.interval)
		defer ticker.Stop()

		o.sample()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				o.sample()
			}
		}
	}()
}

// Stop ends polling and waits for the poller to finish, so the recording is
// stable for the caller to inspect.
func (o *Observer) Stop() {
	if o.cancel == nil {
		return
	}
	o.cancel()
	<-o.done
}

func (o *Observer) sample() {
	round := Round{At: time.Now()}
	for _, e := range o.electors {
		// Read Fence before IsLeader is re-evaluated inside Fence: a node that
		// self-demotes between the two calls reports Fence 0, which is exactly
		// what a downstream store would see.
		if e.IsLeader() {
			round.Claims = append(round.Claims, Claim{Node: e.NodeId(), Fence: e.Fence()})
		}
	}

	o.mu.Lock()
	o.rounds = append(o.rounds, round)
	o.mu.Unlock()
}

// Rounds returns the recording.
func (o *Observer) Rounds() []Round {
	o.mu.Lock()
	defer o.mu.Unlock()

	return append([]Round(nil), o.rounds...)
}

// Overlaps returns the rounds in which more than one node claimed leadership.
// A non-empty result is a mutual-exclusion failure; an empty one is evidence
// that mutual exclusion held for the sampled instants, not proof for all of
// them. See the package doc.
func (o *Observer) Overlaps() []Round {
	var bad []Round
	for _, r := range o.Rounds() {
		if len(r.Claims) > 1 {
			bad = append(bad, r)
		}
	}

	return bad
}

// Terms collapses the recording into the sequence of leadership terms, in
// order. Rounds with no claim separate terms but produce none of their own.
func (o *Observer) Terms() []Term {
	var terms []Term

	for _, r := range o.Rounds() {
		if len(r.Claims) != 1 {
			continue
		}
		c := r.Claims[0]

		if n := len(terms); n > 0 && terms[n-1].Node == c.Node {
			terms[n-1].LastFence = c.Fence
			terms[n-1].Rounds++
			continue
		}

		terms = append(terms, Term{Node: c.Node, FirstFence: c.Fence, LastFence: c.Fence, Rounds: 1})
	}

	return terms
}

// FenceRegression returns a description of the first place the fencing token
// failed to advance, or an empty string when it never did.
//
// The token must never move backwards, and each new term must open strictly
// above the previous term's last observed token. That is the property a
// downstream store relies on when it records the highest token it has accepted
// and rejects anything lower: without it, a zombie leader's late write would
// still be honored.
func (o *Observer) FenceRegression() string {
	terms := o.Terms()

	for i, t := range terms {
		if t.LastFence < t.FirstFence {
			return fmt.Sprintf("term %d (%s) fence moved backwards within the term: %d -> %d",
				i, t.Node, t.FirstFence, t.LastFence)
		}
		if i == 0 {
			continue
		}
		if prev := terms[i-1]; t.FirstFence <= prev.LastFence {
			return fmt.Sprintf("term %d (%s) opened at fence %d, not above term %d (%s) which ended at %d",
				i, t.Node, t.FirstFence, i-1, prev.Node, prev.LastFence)
		}
	}

	return ""
}

// Leaders returns the distinct nodes that held leadership, in order of first
// appearance.
func (o *Observer) Leaders() []string {
	var (
		out  []string
		seen = map[string]struct{}{}
	)
	for _, t := range o.Terms() {
		if _, ok := seen[t.Node]; ok {
			continue
		}
		seen[t.Node] = struct{}{}
		out = append(out, t.Node)
	}

	return out
}

// Timeline renders the recording for a failure message: one line per term plus
// any overlapping rounds, which is enough to see what actually happened without
// dumping every sample.
func (o *Observer) Timeline() string {
	var b strings.Builder

	b.WriteString("terms:\n")
	for i, t := range o.Terms() {
		fmt.Fprintf(&b, "  %d: node=%s fences=%d..%d rounds=%d\n", i, t.Node, t.FirstFence, t.LastFence, t.Rounds)
	}

	overlaps := o.Overlaps()
	if len(overlaps) == 0 {
		return b.String()
	}

	b.WriteString("overlapping rounds:\n")
	for _, r := range overlaps {
		fmt.Fprintf(&b, "  %s:", r.At.Format(time.RFC3339Nano))
		for _, c := range r.Claims {
			fmt.Fprintf(&b, " %s(fence=%d)", c.Node, c.Fence)
		}
		b.WriteByte('\n')
	}

	return b.String()
}
