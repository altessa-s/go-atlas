// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// heldKV reports the election key as held by holder.
type heldKV struct {
	jetstream.KeyValue

	holder string
}

func (k *heldKV) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	return heldEntry{value: k.holder}, nil
}

type heldEntry struct {
	jetstream.KeyValueEntry

	value string
}

func (e heldEntry) Value() []byte { return []byte(e.value) }

func TestLeaderId_AgreesWithIsLeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		leaseAge time.Duration
		want     string
	}{
		{name: "fresh lease answers this node", leaseAge: 0, want: "n"},
		{name: "lease past its freshness bound reads the key", leaseAge: 2 * time.Second, want: "other"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := fakeProvider(&heldKV{holder: "other"}, providers.Config{Key: "k", NodeId: "n", TTL: time.Second})
			renewed := time.Now().Add(-tc.leaseAge)
			p.lastRenewOK.Store(&renewed)
			p.isLeader.Store(true)

			id, err := p.LeaderId(t.Context())
			require.NoError(t, err)
			require.Equal(t, tc.want, id)
			require.Equal(t, p.IsLeader(), id == "n", "LeaderId and IsLeader must agree on whether this node leads")
		})
	}
}

// notifications runs the notification handler of p until the returned stop
// function is called, which makes it deliver what is pending and waits for it.
func notifications(t *testing.T, p *Provider) (stop func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.notificationHandler(t.Context(), p.providerConfig.Load())
	}()
	return func() {
		close(p.notificationCh)
		<-done
	}
}

func TestNotifications_FastFlapDeliversLostThenBecameForTheNewTerm(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		becameCh := make(chan struct{})
		lostCh := make(chan struct{})
		p := fakeProvider(&fakeKV{}, providers.Config{Key: "k", NodeId: "n", TTL: time.Hour, BecameCh: becameCh, LostCh: lostCh})
		defer notifications(t, p)()

		// The handler is delivering the first term and the consumer is not
		// reading yet.
		p.markRenewed(1, time.Now())
		p.setLeader(true)
		synctest.Wait()

		// Leadership is lost and taken again, under a new fencing token,
		// before the handler runs: the state is "leader" before and after.
		p.setLeader(false)
		p.markRenewed(2, time.Now())
		p.setLeader(true)

		receive := func(ch <-chan struct{}, what string) {
			t.Helper()
			synctest.Wait()
			select {
			case <-ch:
			default:
				require.FailNow(t, "notification not delivered", what)
			}
		}
		receive(becameCh, "became leader, first term")
		receive(lostCh, "leadership lost: the term changed")
		receive(becameCh, "became leader, new term")
		require.EqualValues(t, 2, p.Fence(), "the fencing token read after the notification must be the new term's")
	})
}

func TestCampaign_RecreatedKeyWhileLeaderStartsANewTerm(t *testing.T) {
	t.Parallel()

	becameCh := make(chan struct{}, 2)
	lostCh := make(chan struct{}, 2)
	p := fakeProvider(&fakeKV{}, providers.Config{Key: "k", NodeId: "n", TTL: time.Hour, BecameCh: becameCh, LostCh: lostCh})
	c := newCampaign(t, p)
	stop := notifications(t, p)

	c.acquire()
	require.True(t, p.IsLeader())
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return len(becameCh) == 1 }, "the first term was not announced")

	// The key was gone and had to be created again, without this node ever
	// seeing a loss in between.
	c.acquire()
	require.True(t, p.IsLeader())

	stop()
	require.Len(t, becameCh, 2, "the re-created lease is a new term")
	require.Len(t, lostCh, 1, "the consumer must learn that the first term ended")
}

func TestNotifications_StopWhileLeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// becameDelivered tells whether the consumer was told about the term
		// before the provider stopped.
		becameDelivered bool
		wantBecame      int
		wantLost        int
	}{
		{name: "delivered term is ended by one lost", becameDelivered: true, wantBecame: 1, wantLost: 1},
		{name: "undelivered term is never announced after the resignation", becameDelivered: false, wantBecame: 0, wantLost: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			becameCh := make(chan struct{}, 2)
			lostCh := make(chan struct{}, 2)
			p := fakeProvider(&fakeKV{}, providers.Config{Key: "k", NodeId: "n", TTL: time.Hour, BecameCh: becameCh, LostCh: lostCh})

			var stop func()
			if tc.becameDelivered {
				stop = notifications(t, p)
			}
			p.setLeader(true)
			if tc.becameDelivered {
				testhelpers.WaitFor(t, 5*time.Second, func() bool { return len(becameCh) == 1 }, "the term was not announced")
			}

			// What Stop does: mark the provider stopped, then end the loop,
			// which gives up leadership on its way out.
			p.isRunning.Store(false)
			p.setLeader(false)
			p.setLeader(false)

			if !tc.becameDelivered {
				stop = notifications(t, p)
			}
			stop()
			require.Len(t, becameCh, tc.wantBecame)
			require.Len(t, lostCh, tc.wantLost)
		})
	}
}
