// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/reliability/idempotency"

	brokeridem "github.com/altessa-s/go-atlas/messaging/broker/idempotency"
)

var errTransient = errors.New("storage unavailable")

// clockKeeper is an in-memory [brokeridem.Keeper] with an injectable clock and
// a Complete that fails for the first completeFailures calls (all calls when
// completeFailures is negative).
type clockKeeper struct {
	mu               sync.Mutex
	now              time.Time
	maxLock          time.Duration
	records          map[string]*clockRecord
	completeFailures int
	completeCalls    int
}

type clockRecord struct {
	status   idempotency.Status
	lockedAt time.Time
}

func newClockKeeper(completeFailures int) *clockKeeper {
	return &clockKeeper{
		now:              time.Unix(1_700_000_000, 0),
		maxLock:          time.Minute,
		records:          map[string]*clockRecord{},
		completeFailures: completeFailures,
	}
}

func (k *clockKeeper) advance(d time.Duration) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.now = k.now.Add(d)
}

func (k *clockKeeper) calls() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.completeCalls
}

func (k *clockKeeper) AttemptLockWithOpts(_ context.Context, key string, _ idempotency.AttemptLockOpts) (bool, *idempotency.State, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	rec, ok := k.records[key]
	switch {
	case !ok, rec.status == idempotency.StatusInProgress && k.now.Sub(rec.lockedAt) > k.maxLock:
		k.records[key] = &clockRecord{status: idempotency.StatusInProgress, lockedAt: k.now}
		return true, &idempotency.State{Status: idempotency.StatusInProgress}, nil
	default:
		return false, &idempotency.State{Status: rec.status}, nil
	}
}

func (k *clockKeeper) Complete(_ context.Context, key string, _ any, _ *idempotency.State) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.completeCalls++
	if k.completeFailures < 0 || k.completeCalls <= k.completeFailures {
		return errTransient
	}
	k.records[key].status = idempotency.StatusSuccess
	return nil
}

func (k *clockKeeper) Release(_ context.Context, key string, _ *idempotency.State) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.records, key)
	return nil
}

func ackingHandler(runs *int) *testSubscriberHandler {
	return &testSubscriberHandler{topic: "topic", fn: func(_ context.Context, in *msg.Message) {
		*runs++
		_ = in.Ack()
	}}
}

// doubleAckHandler acks the message the handler received (the wrapped one)
// twice.
func doubleAckHandler(second *error) *testSubscriberHandler {
	return &testSubscriberHandler{topic: "topic", fn: func(_ context.Context, in *msg.Message) {
		_ = in.Ack()
		*second = in.Ack()
	}}
}

func dedupMessage(acker msg.Acker, key string) *msg.Message {
	return msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
		{Key: msg.MetaKeyDeduplicateId, Value: key},
	}, msg.WithAcker(acker))
}

// TestWrap_TransientCompleteFailureDoesNotRerunHandler verifies that a
// transient Complete failure is retried within the release timeout, so the
// record turns SUCCESS and a duplicate arriving after MaxLockDuration is not
// handed to the handler again.
func TestWrap_TransientCompleteFailureDoesNotRerunHandler(t *testing.T) {
	t.Parallel()

	keeper := newClockKeeper(2)
	var runs int
	wrapped := brokeridem.Wrap(ackingHandler(&runs), keeper, "consumer")

	first := &mockAcker{}
	wrapped.Handle(t.Context(), dedupMessage(first, "k"))
	require.True(t, first.acked)
	require.Equal(t, 1, runs)

	keeper.advance(2 * time.Minute)

	copyAcker := &mockAcker{}
	wrapped.Handle(t.Context(), dedupMessage(copyAcker, "k"))
	require.Equal(t, 1, runs, "a duplicate after MaxLockDuration must not run the handler again")
	require.True(t, copyAcker.acked)
}

// TestWrap_PersistentCompleteFailureAcks verifies that when Complete keeps
// failing the effect is already applied, so the message is Acked (never
// Nak'd) and the failure is counted once; a second Ack of the handler's
// message neither completes again nor counts again.
func TestWrap_PersistentCompleteFailureAcks(t *testing.T) {
	t.Parallel()

	keeper := newClockKeeper(-1)
	tc := metricstest.NewTestCollector()
	var secondAck error
	wrapped := brokeridem.Wrap(doubleAckHandler(&secondAck), keeper, "consumer", brokeridem.WithMeter(tc))

	acker := &mockAcker{}
	wrapped.Handle(t.Context(), dedupMessage(acker, "k"))

	require.True(t, acker.acked)
	require.False(t, acker.naked)
	require.NoError(t, secondAck)
	completeCalls := keeper.calls()
	require.Greater(t, completeCalls, 1, "Complete must be retried")
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, tc, "test_broker_idempotency_outcomes_total",
		"consumer", "consumer", "outcome", "complete_failed"))
}

// TestWrap_LogsDoNotContainBusinessKey verifies that failure logs carry
// neither the business key nor the storage key composed from it.
func TestWrap_LogsDoNotContainBusinessKey(t *testing.T) {
	t.Parallel()

	const email = "alice@example.com"
	leaky := errors.New("storage error for key idk:v2:3:ibk8:consumer5:topic17:" + email)
	tests := []struct {
		name   string
		keeper *fakeKeeper
	}{
		{"lock failure", &fakeKeeper{lockErr: leaky}},
		{"complete failure", &fakeKeeper{acquired: true, state: &idempotency.State{}, complete: leaky}},
		{"release failure", &fakeKeeper{acquired: true, state: &idempotency.State{}, release: leaky}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))
			h := &testSubscriberHandler{topic: "topic", fn: func(_ context.Context, in *msg.Message) {
				if tc.keeper.release != nil {
					_ = in.Nak()
					return
				}
				_ = in.Ack()
			}}
			wrapped := brokeridem.Wrap(h, tc.keeper, "consumer", brokeridem.WithLogger(logger))
			wrapped.Handle(t.Context(), dedupMessage(&mockAcker{}, email))
			require.NotEmpty(t, buf.String(), "the failure must be logged")
			require.NotContains(t, buf.String(), email)
		})
	}
}

// TestWrap_ExtractorErrorTextStaysOutOfLogsAndTerm verifies that an error from
// the key extractor, which may echo message content, reaches neither the logs
// nor the broker.
func TestWrap_ExtractorErrorTextStaysOutOfLogsAndTerm(t *testing.T) {
	t.Parallel()

	const secret = "alice@example.com"
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	h := &testSubscriberHandler{topic: "topic"}
	wrapped := brokeridem.Wrap(h, &fakeKeeper{}, "consumer",
		brokeridem.WithLogger(logger),
		brokeridem.WithKeyFunc(func(*msg.Message) (string, error) { return "", errors.New("cannot parse " + secret) }),
	)
	acker := &mockAcker{}
	wrapped.Handle(t.Context(), dedupMessage(acker, "k"))

	require.True(t, acker.termed)
	require.NotContains(t, acker.termReason, secret)
	require.NotContains(t, buf.String(), secret)
	require.NotEmpty(t, buf.String())
}
