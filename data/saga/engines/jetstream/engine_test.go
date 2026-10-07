// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jetstream_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/memory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	sagajs "github.com/altessa-s/go-atlas/data/saga/engines/jetstream"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
	natsstore "github.com/altessa-s/go-atlas/data/saga/storages/nats"
)

const waitTimeout = 10 * time.Second

var errStep = errors.New("step failed")

type order struct {
	N int `json:"n"`
}

// env is one test's isolated JetStream server plus a saga store.
type env struct {
	js    jetstream.JetStream
	store *memory.Store
	close func()
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ns := testhelpers.StartNATSServer(t)
	nc, js := testhelpers.ConnectJetStream(t, ns)
	return &env{js: js, store: memory.New(), close: nc.Close}
}

// definition builds a two-step saga whose second step runs fn.
func definition(name string, fn saga.StepFunc[order]) *saga.Definition[order] {
	noop := func(context.Context, *order) error { return nil }
	return saga.NewDefinition[order](name).
		Step("a", func(_ context.Context, o *order) error { o.N++; return nil }).Compensate(noop).
		Step("b", fn).Compensate(noop).
		MustBuild()
}

func ok(context.Context, *order) error { return nil }

func newEngine(t *testing.T, e *env, store saga.Storage, def *saga.Definition[order], opts ...sagajs.Option) *sagajs.Engine[order] {
	t.Helper()
	orch := saga.New(store, def, saga.WithMaxStepAttempts(1), saga.WithStepRetryBaseDelay(time.Millisecond))
	engine, err := sagajs.New(t.Context(), e.js, orch, append([]sagajs.Option{sagajs.WithRetryBaseDelay(10 * time.Millisecond)}, opts...)...)
	require.NoError(t, err)
	return engine
}

// run starts engine.Run and returns a stop function that cancels it and
// returns Run's result.
func run(t *testing.T, engine *sagajs.Engine[order]) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx) }()
	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(waitTimeout):
			t.Fatal("Run did not return after cancellation")
			return nil
		}
	}
}

func waitStatus(t *testing.T, store saga.Storage, id string, want saga.Status) {
	t.Helper()
	testhelpers.WaitFor(t, waitTimeout, func() bool {
		inst, err := store.Get(t.Context(), id)
		return err == nil && inst.Status == want
	}, "saga "+id+" did not reach "+string(want))
}

func streamMsgs(t *testing.T, js jetstream.JetStream) uint64 {
	t.Helper()
	s, err := js.Stream(t.Context(), sagajs.DefaultStream)
	require.NoError(t, err)
	info, err := s.Info(t.Context())
	require.NoError(t, err)
	return info.State.Msgs
}

func TestSubmitRunCompletes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tc := testhelpers.NewTestCollector()
	engine := newEngine(t, e, e.store, definition("place", ok), sagajs.WithCollector(tc))
	stop := run(t, engine)

	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	waitStatus(t, e.store, "o1", saga.StatusCompleted)
	testhelpers.WaitFor(t, waitTimeout, func() bool { return streamMsgs(t, e.js) == 0 }, "command was not acked")
	require.NoError(t, stop())

	inst, err := e.store.Get(t.Context(), "o1")
	require.NoError(t, err)
	require.JSONEq(t, `{"n":1}`, string(inst.Data))
	require.InDelta(t, 1, testhelpers.GetCounterValue(t, tc, "test_saga_engine_submitted_total"), 0.001)
	require.InDelta(t, 1, testhelpers.GetCounterValue(t, tc, "test_saga_engine_acked_total"), 0.001)
}

func TestCompensatedOutcomeIsAcked(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	engine := newEngine(t, e, e.store, definition("place", func(context.Context, *order) error { return errStep }))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	waitStatus(t, e.store, "o1", saga.StatusCompensated)
	testhelpers.WaitFor(t, waitTimeout, func() bool { return streamMsgs(t, e.js) == 0 }, "business failure must be acked, not redelivered")
}

func TestDuplicateSubmitIsCollapsed(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	engine := newEngine(t, e, e.store, definition("place", ok))

	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	require.Equal(t, uint64(1), streamMsgs(t, e.js))
}

func TestSameIDAcrossDefinitionsIsNotCollapsed(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a := newEngine(t, e, e.store, definition("place", ok))
	b := newEngine(t, e, memory.New(), definition("refund", ok))

	require.NoError(t, a.Submit(t.Context(), "same", order{}))
	require.NoError(t, b.Submit(t.Context(), "same", order{}))
	require.Equal(t, uint64(2), streamMsgs(t, e.js))
}

func TestSubmitRejectsEmptyID(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	engine := newEngine(t, e, e.store, definition("place", ok))
	require.ErrorIs(t, engine.Submit(t.Context(), "", order{}), sagaerrs.ErrEmptyID)
}

// failOnceStore fails the first Update whose instance matches, simulating a
// one-off store outage at a chosen checkpoint.
type failOnceStore struct {
	saga.Storage
	match  func(*saga.Instance) bool
	failed atomic.Bool
}

func (s *failOnceStore) Update(ctx context.Context, inst *saga.Instance) error {
	if s.match(inst) && s.failed.CompareAndSwap(false, true) {
		return errors.New("store outage")
	}
	return s.Storage.Update(ctx, inst)
}

// failTerminalOnce fails the first write of a terminal status, so the saga is
// interrupted after its last step committed.
func failTerminalOnce(store saga.Storage) *failOnceStore {
	return &failOnceStore{Storage: store, match: func(inst *saga.Instance) bool { return inst.Status.IsTerminal() }}
}

// failFirstUpdate fails the very first Update (the lease acquisition), so the
// first delivery is interrupted before any step runs.
func failFirstUpdate(store saga.Storage) *failOnceStore {
	return &failOnceStore{Storage: store, match: func(*saga.Instance) bool { return true }}
}

func TestInterruptedExecutionIsRedelivered(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	store := failTerminalOnce(e.store)
	tc := testhelpers.NewTestCollector()
	engine := newEngine(t, e, store, definition("place", ok), sagajs.WithCollector(tc))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	waitStatus(t, e.store, "o1", saga.StatusCompleted)
	require.True(t, store.failed.Load())
	require.GreaterOrEqual(t, testhelpers.GetCounterValue(t, tc, "test_saga_engine_naked_total"), 1.0)
}

func TestPoisonCommandIsTerminated(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tc := testhelpers.NewTestCollector()
	engine := newEngine(t, e, e.store, definition("place", ok), sagajs.WithCollector(tc))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	// Undecodable envelope, missing ID, a valid ID ("x") with data that is not
	// JSON, a missing data field, and empty data the JSON serializer rejects.
	for _, body := range []string{`not json`, `{"data":"e30="}`, `{"id":"x","data":"bm90IGpzb24="}`, `{"id":"x"}`, `{"id":"x","data":""}`} {
		_, err := e.js.Publish(t.Context(), sagajs.DefaultSubjectPrefix+".place", []byte(body))
		require.NoError(t, err)
	}
	testhelpers.WaitFor(t, waitTimeout, func() bool {
		return testhelpers.GetCounterValue(t, tc, "test_saga_engine_terminated_total") == 5
	}, "poison commands were not terminated")
	require.Equal(t, uint64(0), streamMsgs(t, e.js))
	_, err := e.store.Get(t.Context(), "x")
	require.Error(t, err)
}

// slowStep runs for d unless canceled and counts its invocations.
func slowStep(calls *atomic.Int32, d time.Duration) saga.StepFunc[order] {
	return func(ctx context.Context, _ *order) error {
		calls.Add(1)
		select {
		case <-time.After(d):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func consumed(t *testing.T, cs ...*testhelpers.TestCollector) float64 {
	t.Helper()
	var n float64
	for _, c := range cs {
		n += testhelpers.GetCounterValue(t, c, "test_saga_engine_consumed_total")
	}
	return n
}

func TestLongExecutionIsNotRedelivered(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var calls atomic.Int32
	// Two engines compete; a missed heartbeat would hand the command to the
	// other one, which shows up as a second delivery (the lease would hide it
	// from the step counter, so deliveries are what is asserted).
	ca, cb := testhelpers.NewTestCollector(), testhelpers.NewTestCollector()
	first := newEngine(t, e, e.store, definition("place", slowStep(&calls, 2*time.Second)),
		sagajs.WithAckWait(600*time.Millisecond), sagajs.WithCollector(ca))
	second := newEngine(t, e, e.store, definition("place", slowStep(&calls, 2*time.Second)), sagajs.WithCollector(cb))
	stopFirst, stopSecond := run(t, first), run(t, second)
	defer func() { require.NoError(t, stopFirst()); require.NoError(t, stopSecond()) }()

	require.NoError(t, first.Submit(t.Context(), "o1", order{}))
	waitStatus(t, e.store, "o1", saga.StatusCompleted)
	require.InDelta(t, 1, consumed(t, ca, cb), 0.001)
	require.Equal(t, int32(1), calls.Load())
}

func TestExistingConsumerAckWaitIsAdopted(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var calls atomic.Int32
	def := definition("place", slowStep(&calls, 2*time.Second))
	// The consumer is created with a short AckWait; the engine that runs asks
	// for 30s and must heartbeat against the real 600ms deadline instead.
	newEngine(t, e, e.store, def, sagajs.WithAckWait(600*time.Millisecond))
	tc := testhelpers.NewTestCollector()
	engine := newEngine(t, e, e.store, def, sagajs.WithAckWait(30*time.Second), sagajs.WithCollector(tc))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	waitStatus(t, e.store, "o1", saga.StatusCompleted)
	require.InDelta(t, 1, consumed(t, tc), 0.001)
}

func TestTinyAckWaitDoesNotPanic(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	engine := newEngine(t, e, e.store, definition("place", ok), sagajs.WithAckWait(time.Nanosecond))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	waitStatus(t, e.store, "o1", saga.StatusCompleted)
}

// panickySerializer panics while decoding, like a buggy T.UnmarshalJSON.
type panickySerializer struct{ serializer.JSON }

func (panickySerializer) Deserialize([]byte, any) error { panic("boom") }

func TestDecodePanicIsRedelivered(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tc := testhelpers.NewTestCollector()
	engine := newEngine(t, e, e.store, definition("place", ok),
		sagajs.WithSerializer(&panickySerializer{}), sagajs.WithCollector(tc))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	require.NoError(t, engine.Submit(t.Context(), "o1", order{N: 1}))
	testhelpers.WaitFor(t, waitTimeout, func() bool {
		return testhelpers.GetCounterValue(t, tc, "test_saga_engine_naked_total") >= 1
	}, "panicking decode was not returned for redelivery")
	require.Equal(t, uint64(1), streamMsgs(t, e.js), "the command must stay queued")
}

func TestSharedStoreIDCollisionIsTerminated(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tc := testhelpers.NewTestCollector()
	place := newEngine(t, e, e.store, definition("place", ok))
	refund := newEngine(t, e, e.store, definition("refund", ok), sagajs.WithCollector(tc))

	require.NoError(t, place.Submit(t.Context(), "same", order{}))
	stopPlace := run(t, place)
	defer func() { require.NoError(t, stopPlace()) }()
	waitStatus(t, e.store, "same", saga.StatusCompleted)

	require.NoError(t, refund.Submit(t.Context(), "same", order{}))
	stopRefund := run(t, refund)
	defer func() { require.NoError(t, stopRefund()) }()
	testhelpers.WaitFor(t, waitTimeout, func() bool {
		return testhelpers.GetCounterValue(t, tc, "test_saga_engine_terminated_total") == 1
	}, "colliding command was not terminated")
	inst, err := e.store.Get(t.Context(), "same")
	require.NoError(t, err)
	require.Equal(t, "place", inst.Definition)
}

func TestConcurrencyIsBounded(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var cur, peak atomic.Int32
	step := func(context.Context, *order) error {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		cur.Add(-1)
		return nil
	}
	engine := newEngine(t, e, e.store, definition("place", step), sagajs.WithConcurrency(2))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	ids := []string{"o1", "o2", "o3", "o4", "o5", "o6"}
	for _, id := range ids {
		require.NoError(t, engine.Submit(t.Context(), id, order{}))
	}
	for _, id := range ids {
		waitStatus(t, e.store, id, saga.StatusCompleted)
	}
	require.Equal(t, int32(2), peak.Load())
}

// blockingStep blocks until its context is canceled while block is set.
func blockingStep(block *atomic.Bool, entered chan<- struct{}) saga.StepFunc[order] {
	return func(ctx context.Context, _ *order) error {
		if !block.Load() {
			return nil
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return ctx.Err()
	}
}

func TestShutdownRedeliversInterruptedExecution(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var block atomic.Bool
	block.Store(true)
	entered := make(chan struct{}, 1)
	engine := newEngine(t, e, e.store, definition("place", blockingStep(&block, entered)))
	stop := run(t, engine)

	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	<-entered
	require.NoError(t, stop())
	inst, err := e.store.Get(t.Context(), "o1")
	require.NoError(t, err)
	require.False(t, inst.Status.IsTerminal())

	block.Store(false)
	stop = run(t, engine)
	defer func() { require.NoError(t, stop()) }()
	waitStatus(t, e.store, "o1", saga.StatusCompleted)
}

func TestConnectionCloseStopsRun(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var block atomic.Bool
	block.Store(true)
	entered := make(chan struct{}, 1)
	// One slot, held by a blocked execution: Run waits for a free slot and the
	// second command stays on the server.
	engine := newEngine(t, e, e.store, definition("place", blockingStep(&block, entered)), sagajs.WithConcurrency(1))
	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	require.NoError(t, engine.Submit(t.Context(), "o2", order{}))

	done := make(chan error, 1)
	go func() { done <- engine.Run(t.Context()) }()
	<-entered
	time.Sleep(100 * time.Millisecond) // let Run block waiting for the slot
	e.close()

	select {
	case err := <-done:
		require.ErrorIs(t, err, sagajs.ErrConsumeStopped)
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return after the connection closed")
	}
}

func TestRunRejectsConcurrentRun(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	engine := newEngine(t, e, e.store, definition("place", ok))

	// Whichever Run takes the engine holds it until cancel, so the other one
	// must be rejected — regardless of which goroutine starts first.
	ctx, cancel := context.WithCancel(t.Context())
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- engine.Run(ctx) }()
	}
	select {
	case err := <-results:
		require.ErrorIs(t, err, sagajs.ErrAlreadyRunning)
	case <-time.After(waitTimeout):
		t.Fatal("neither Run was rejected")
	}
	cancel()
	require.NoError(t, <-results)
}

func TestNewValidation(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	orch := saga.New(e.store, definition("place", ok))

	_, err := sagajs.New[order](t.Context(), nil, orch)
	require.Error(t, err)
	_, err = sagajs.New[order](t.Context(), e.js, nil)
	require.Error(t, err)
	_, err = sagajs.New(t.Context(), e.js, saga.New(e.store, definition("bad.name", ok)))
	require.ErrorIs(t, err, sagajs.ErrInvalidSubject)
	_, err = sagajs.New(t.Context(), e.js, orch, sagajs.WithSubjectPrefix("saga.*"))
	require.ErrorIs(t, err, sagajs.ErrInvalidSubject)
}

func TestNewRejectsIncompatibleResources(t *testing.T) {
	t.Parallel()

	t.Run("limits_stream", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		_, err := e.js.CreateStream(t.Context(), jetstream.StreamConfig{Name: sagajs.DefaultStream, Subjects: []string{"saga.start.>"}})
		require.NoError(t, err)
		_, err = sagajs.New(t.Context(), e.js, saga.New(e.store, definition("place", ok)))
		require.ErrorIs(t, err, sagajs.ErrIncompatibleStream)
	})

	t.Run("uncovered_subject", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		_, err := e.js.CreateStream(t.Context(), jetstream.StreamConfig{
			Name: sagajs.DefaultStream, Subjects: []string{"saga.start.other"}, Retention: jetstream.WorkQueuePolicy,
		})
		require.NoError(t, err)
		_, err = sagajs.New(t.Context(), e.js, saga.New(e.store, definition("place", ok)))
		require.ErrorIs(t, err, sagajs.ErrIncompatibleStream)
	})

	t.Run("subject_transform", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		_, err := e.js.CreateStream(t.Context(), jetstream.StreamConfig{
			Name: sagajs.DefaultStream, Subjects: []string{"saga.start.>"}, Retention: jetstream.WorkQueuePolicy,
			SubjectTransform: &jetstream.SubjectTransformConfig{Source: "saga.start.>", Destination: "moved.>"},
		})
		require.NoError(t, err)
		_, err = sagajs.New(t.Context(), e.js, saga.New(e.store, definition("place", ok)))
		require.ErrorIs(t, err, sagajs.ErrIncompatibleStream)
	})

	t.Run("no_ack", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		_, err := e.js.CreateStream(t.Context(), jetstream.StreamConfig{
			Name: sagajs.DefaultStream, Subjects: []string{"saga.start.>"}, Retention: jetstream.WorkQueuePolicy, NoAck: true,
		})
		require.NoError(t, err)
		_, err = sagajs.New(t.Context(), e.js, saga.New(e.store, definition("place", ok)))
		require.ErrorIs(t, err, sagajs.ErrIncompatibleStream)
	})

	t.Run("foreign_consumer", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		s, err := e.js.CreateStream(t.Context(), jetstream.StreamConfig{
			Name: sagajs.DefaultStream, Subjects: []string{"saga.start.>"}, Retention: jetstream.WorkQueuePolicy,
		})
		require.NoError(t, err)
		_, err = s.CreateConsumer(t.Context(), jetstream.ConsumerConfig{
			Durable: "saga-place", FilterSubject: "saga.start.other", AckPolicy: jetstream.AckExplicitPolicy,
		})
		require.NoError(t, err)
		_, err = sagajs.New(t.Context(), e.js, saga.New(e.store, definition("place", ok)))
		require.ErrorIs(t, err, sagajs.ErrIncompatibleConsumer)
	})
}

func TestExistingResourcesAreNotModified(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	newEngine(t, e, e.store, definition("place", ok), sagajs.WithAckWait(2*time.Second))
	newEngine(t, e, e.store, definition("place", ok), sagajs.WithAckWait(5*time.Second), sagajs.WithMaxAge(time.Minute))

	s, err := e.js.Stream(t.Context(), sagajs.DefaultStream)
	require.NoError(t, err)
	require.Zero(t, s.CachedInfo().Config.MaxAge)
	c, err := s.Consumer(t.Context(), "saga-place")
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, c.CachedInfo().Config.AckWait)
}

func TestConcurrentNewSucceeds(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() {
			orch := saga.New(e.store, definition("place", ok))
			_, errs[i] = sagajs.New(t.Context(), e.js, orch, sagajs.WithAckWait(time.Duration(i+1)*time.Second))
		})
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
}

func TestWhitespaceIDsAreNotCollapsed(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	engine := newEngine(t, e, e.store, definition("place", ok))

	// Header values are normalized on the wire; the de-duplication key must
	// still tell these IDs apart.
	for _, id := range []string{"order", "order ", "order\r\n", "a\r\nb", "a b"} {
		require.NoError(t, engine.Submit(t.Context(), id, order{}))
	}
	require.Equal(t, uint64(5), streamMsgs(t, e.js))
}

func TestHeadersOnlyConsumerIsRejected(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	engine := newEngine(t, e, e.store, definition("place", ok))
	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))

	s, err := e.js.Stream(t.Context(), sagajs.DefaultStream)
	require.NoError(t, err)
	require.NoError(t, s.DeleteConsumer(t.Context(), "saga-place"))
	_, err = s.CreateConsumer(t.Context(), jetstream.ConsumerConfig{
		Durable: "saga-place", FilterSubject: "saga.start.place", AckPolicy: jetstream.AckExplicitPolicy, HeadersOnly: true,
	})
	require.NoError(t, err)

	_, err = sagajs.New(t.Context(), e.js, saga.New(e.store, definition("place", ok)))
	require.ErrorIs(t, err, sagajs.ErrIncompatibleConsumer)
	require.Equal(t, uint64(1), streamMsgs(t, e.js), "the queued command must be left intact")
}

func TestConsumerDeletionStopsRun(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	engine := newEngine(t, e, e.store, definition("place", ok))
	done := make(chan error, 1)
	go func() { done <- engine.Run(t.Context()) }()

	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	waitStatus(t, e.store, "o1", saga.StatusCompleted)
	s, err := e.js.Stream(t.Context(), sagajs.DefaultStream)
	require.NoError(t, err)
	require.NoError(t, s.DeleteConsumer(t.Context(), "saga-place"))

	select {
	case err := <-done:
		require.ErrorIs(t, err, sagajs.ErrConsumeStopped)
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return after the consumer was deleted")
	}
}

// precreate provisions the stream and a consumer with the given settings, as
// an operator (or an older deployment) would before the engine starts.
func precreate(t *testing.T, js jetstream.JetStream, cfg jetstream.ConsumerConfig) {
	t.Helper()
	s, err := js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name: sagajs.DefaultStream, Subjects: []string{"saga.start.>"}, Retention: jetstream.WorkQueuePolicy,
	})
	require.NoError(t, err)
	cfg.Durable, cfg.FilterSubject, cfg.AckPolicy = "saga-place", "saga.start.place", jetstream.AckExplicitPolicy
	_, err = s.CreateConsumer(t.Context(), cfg)
	require.NoError(t, err)
}

func TestConsumerPullLimitsAreHonored(t *testing.T) {
	t.Parallel()
	for name, cfg := range map[string]jetstream.ConsumerConfig{
		"max_request_batch":   {MaxRequestBatch: 1},
		"max_request_expires": {MaxRequestExpires: time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			precreate(t, e.js, cfg)
			engine := newEngine(t, e, e.store, definition("place", ok))
			stop := run(t, engine)
			defer func() { require.NoError(t, stop()) }()

			for _, id := range []string{"o1", "o2", "o3"} {
				require.NoError(t, engine.Submit(t.Context(), id, order{}))
			}
			for _, id := range []string{"o1", "o2", "o3"} {
				waitStatus(t, e.store, id, saga.StatusCompleted)
			}
		})
	}
}

func TestShortBackOffRedeliveryIsKeptAlive(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// The first delivery may take 30s, a redelivery only 300ms. The engine
	// must heartbeat against the shorter deadline or the redelivered, running
	// command is handed out again.
	precreate(t, e.js, jetstream.ConsumerConfig{BackOff: []time.Duration{30 * time.Second, 300 * time.Millisecond}, MaxDeliver: 10})
	var calls atomic.Int32
	tc := testhelpers.NewTestCollector()
	store := failFirstUpdate(e.store)
	engine := newEngine(t, e, store, definition("place", slowStep(&calls, time.Second)), sagajs.WithCollector(tc), sagajs.WithConcurrency(2))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	waitStatus(t, e.store, "o1", saga.StatusCompleted)
	require.InDelta(t, 2, consumed(t, tc), 0.001, "one interrupted delivery plus one that completes")
}

func TestConnectionCloseDuringPartialFetch(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var block atomic.Bool
	block.Store(true)
	entered := make(chan struct{}, 1)
	// Default concurrency: one running saga, and a fetch waiting for more.
	engine := newEngine(t, e, e.store, definition("place", blockingStep(&block, entered)))
	done := make(chan error, 1)
	go func() { done <- engine.Run(t.Context()) }()
	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	<-entered

	start := time.Now()
	e.close()
	select {
	case err := <-done:
		require.ErrorIs(t, err, sagajs.ErrConsumeStopped)
		require.Less(t, time.Since(start), 2*time.Second, "closure must not wait for the fetch window")
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return after the connection closed")
	}
}

func TestConsumerDeletionWhileSaturatedStopsRun(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	release, entered := make(chan struct{}), make(chan struct{}, 1)
	step := func(ctx context.Context, _ *order) error {
		entered <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// The only slot is busy, so no pull request is outstanding when the
	// consumer is deleted and the server sends no deletion notice.
	engine := newEngine(t, e, e.store, definition("place", step), sagajs.WithConcurrency(1))
	done := make(chan error, 1)
	go func() { done <- engine.Run(t.Context()) }()
	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	<-entered

	s, err := e.js.Stream(t.Context(), sagajs.DefaultStream)
	require.NoError(t, err)
	require.NoError(t, s.DeleteConsumer(t.Context(), "saga-place"))
	close(release)

	select {
	case err := <-done:
		require.ErrorIs(t, err, sagajs.ErrConsumeStopped)
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return after the consumer was deleted")
	}
	waitStatus(t, e.store, "o1", saga.StatusCompleted)
}

// drainHandler is a slog handler that closes draining when the engine logs
// that it noticed the consumer is gone and is finishing in-flight executions.
type drainHandler struct {
	once     *sync.Once
	draining chan struct{}
}

func (drainHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h drainHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "saga engine: consumer gone; finishing in-flight executions" {
		h.once.Do(func() { close(h.draining) })
	}
	return nil
}

func (h drainHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h drainHandler) WithGroup(string) slog.Handler      { return h }

// deletionFixture runs an engine with two slots, one saga blocked mid-step and
// the free slot keeping a pull request outstanding, then deletes the consumer.
// draining closes once Run has noticed the deletion.
type deletionFixture struct {
	e        *env
	release  chan struct{}
	draining chan struct{}
	canceled atomic.Bool
	cancel   context.CancelFunc
	done     chan error
}

func startDeletionFixture(t *testing.T) *deletionFixture {
	t.Helper()
	f := &deletionFixture{e: newEnv(t), release: make(chan struct{}), draining: make(chan struct{}), done: make(chan error, 1)}
	entered := make(chan struct{}, 1)
	step := func(ctx context.Context, _ *order) error {
		entered <- struct{}{}
		select {
		case <-f.release:
			return nil
		case <-ctx.Done():
			f.canceled.Store(true)
			return ctx.Err()
		}
	}
	logger := slog.New(drainHandler{once: &sync.Once{}, draining: f.draining})
	engine := newEngine(t, f.e, f.e.store, definition("place", step), sagajs.WithConcurrency(2), sagajs.WithLogger(logger))
	ctx, cancel := context.WithCancel(t.Context())
	f.cancel = cancel
	go func() { f.done <- engine.Run(ctx) }()
	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	<-entered

	s, err := f.e.js.Stream(t.Context(), sagajs.DefaultStream)
	require.NoError(t, err)
	require.NoError(t, s.DeleteConsumer(t.Context(), "saga-place"))
	select {
	case <-f.draining:
	case <-time.After(waitTimeout):
		t.Fatal("Run did not notice the consumer deletion")
	}
	return f
}

func (f *deletionFixture) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-f.done:
		return err
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return")
		return nil
	}
}

func TestConsumerDeletionDrainsRunningExecutions(t *testing.T) {
	t.Parallel()
	f := startDeletionFixture(t)
	require.Never(t, func() bool { return f.canceled.Load() || len(f.done) > 0 }, 300*time.Millisecond, 10*time.Millisecond,
		"the deletion must neither cancel the running saga nor end Run before it finishes")
	close(f.release)

	err := f.wait(t)
	require.ErrorIs(t, err, sagajs.ErrConsumeStopped)
	require.True(t, errors.Is(err, jetstream.ErrConsumerDeleted) || errors.Is(err, jetstream.ErrConsumerNotFound), "cause: %v", err)
	inst, err := f.e.store.Get(t.Context(), "o1")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status, "Run returns only after the saga persisted its outcome")
}

func TestConsumerDeletionDrainIsBoundedByContext(t *testing.T) {
	t.Parallel()
	f := startDeletionFixture(t)
	f.cancel()

	require.ErrorIs(t, f.wait(t), sagajs.ErrConsumeStopped, "consumption had already stopped before the cancellation")
	require.True(t, f.canceled.Load(), "canceling the context must cancel the draining saga")
	inst, err := f.e.store.Get(t.Context(), "o1")
	require.NoError(t, err)
	require.False(t, inst.Status.IsTerminal())
}

func TestInvalidUTF8IDIsRejected(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tc := testhelpers.NewTestCollector()
	engine := newEngine(t, e, e.store, definition("place", ok), sagajs.WithCollector(tc))
	require.ErrorIs(t, engine.Submit(t.Context(), "order\xff", order{}), sagajs.ErrInvalidID)

	// A raw command with such an ID is poison, not a saga.
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()
	_, err := e.js.Publish(t.Context(), sagajs.DefaultSubjectPrefix+".place", []byte("{\"id\":\"order\xff\",\"data\":\"e30=\"}"))
	require.NoError(t, err)
	testhelpers.WaitFor(t, waitTimeout, func() bool {
		return testhelpers.GetCounterValue(t, tc, "test_saga_engine_terminated_total") == 1
	}, "invalid UTF-8 ID was not terminated")
}

func TestNATSKVInvalidKeyIsTerminated(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	store, err := natsstore.New(e.js, natsstore.WithBucket("saga_ids"))
	require.NoError(t, err)
	tc := testhelpers.NewTestCollector()
	engine := newEngine(t, e, store, definition("place", ok), sagajs.WithCollector(tc))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	// Valid UTF-8, but not a valid NATS KV key: retrying could never succeed.
	require.NoError(t, engine.Submit(t.Context(), "order 1", order{}))
	testhelpers.WaitFor(t, waitTimeout, func() bool {
		return testhelpers.GetCounterValue(t, tc, "test_saga_engine_terminated_total") == 1
	}, "an ID the KV store rejects must be terminated, not retried")
	require.Equal(t, uint64(0), streamMsgs(t, e.js))
}

func TestInterruptedExecutionResumesOnNATSKV(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	kv, err := natsstore.New(e.js, natsstore.WithBucket("saga_resume"))
	require.NoError(t, err)
	store := failTerminalOnce(kv)
	engine := newEngine(t, e, store, definition("place", ok))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	require.NoError(t, engine.Submit(t.Context(), "order-1", order{}))
	waitStatus(t, kv, "order-1", saga.StatusCompleted)
	require.True(t, store.failed.Load(), "the first terminal write was interrupted")
}

// slowSerializer decodes slower than the consumer's AckWait.
type slowSerializer struct{ serializer.JSON }

func (s *slowSerializer) Deserialize(b []byte, out any) error {
	time.Sleep(1500 * time.Millisecond)
	return s.JSON.Deserialize(b, out)
}

func TestSlowDecodeIsNotRedelivered(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ca, cb := testhelpers.NewTestCollector(), testhelpers.NewTestCollector()
	first := newEngine(t, e, e.store, definition("place", ok),
		sagajs.WithAckWait(600*time.Millisecond), sagajs.WithSerializer(&slowSerializer{}), sagajs.WithCollector(ca))
	second := newEngine(t, e, e.store, definition("place", ok), sagajs.WithSerializer(&slowSerializer{}), sagajs.WithCollector(cb))
	stopFirst, stopSecond := run(t, first), run(t, second)
	defer func() { require.NoError(t, stopFirst()); require.NoError(t, stopSecond()) }()

	require.NoError(t, first.Submit(t.Context(), "o1", order{}))
	waitStatus(t, e.store, "o1", saga.StatusCompleted)
	require.InDelta(t, 1, consumed(t, ca, cb), 0.001)
}

// emptyOKSerializer encodes every value as zero bytes (nil when nilBytes is
// set) and decodes zero bytes into a meaningful value, which the engine must
// not skip.
type emptyOKSerializer struct{ nilBytes bool }

func (s emptyOKSerializer) Serialize(any) ([]byte, error) {
	if s.nilBytes {
		return nil, nil
	}
	return []byte{}, nil
}

func (emptyOKSerializer) Deserialize(b []byte, out any) error {
	if len(b) == 0 {
		out.(*order).N = 41
	}
	return nil
}

func TestEmptyPayloadIsStillDecoded(t *testing.T) {
	t.Parallel()
	for name, nilBytes := range map[string]bool{"empty_slice": false, "nil_slice": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			engine := newEngine(t, e, e.store, definition("place", ok), sagajs.WithSerializer(emptyOKSerializer{nilBytes: nilBytes}))
			stop := run(t, engine)
			defer func() { require.NoError(t, stop()) }()

			require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
			waitStatus(t, e.store, "o1", saga.StatusCompleted)
			inst, err := e.store.Get(t.Context(), "o1")
			require.NoError(t, err)
			require.JSONEq(t, `{"n":42}`, string(inst.Data), "the decoded empty payload (41) plus step a")
		})
	}
}

// A producer following the documented envelope — a literal JSON string ID and
// base64 data — is understood without going through Submit.
func TestLiteralEnvelopeIsAccepted(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	engine := newEngine(t, e, e.store, definition("place", ok))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	_, err := e.js.Publish(t.Context(), sagajs.DefaultSubjectPrefix+".place", []byte(`{"id":"order-1","data":"eyJuIjo0MX0="}`)) // {"n":41}
	require.NoError(t, err)
	waitStatus(t, e.store, "order-1", saga.StatusCompleted)
	inst, err := e.store.Get(t.Context(), "order-1")
	require.NoError(t, err)
	require.JSONEq(t, `{"n":42}`, string(inst.Data))
	_, err = e.store.Get(t.Context(), "YWJj")
	require.Error(t, err, "the ID is a plain string, never base64-decoded")
}

func TestInterruptedCompensationIsRedelivered(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	store := failTerminalOnce(e.store)
	engine := newEngine(t, e, store, definition("place", func(context.Context, *order) error { return errStep }))
	stop := run(t, engine)
	defer func() { require.NoError(t, stop()) }()

	require.NoError(t, engine.Submit(t.Context(), "o1", order{}))
	waitStatus(t, e.store, "o1", saga.StatusCompensated)
	require.True(t, store.failed.Load(), "the first Compensated write was interrupted")
	testhelpers.WaitFor(t, waitTimeout, func() bool { return streamMsgs(t, e.js) == 0 }, "command was not acked after recovery")
}
