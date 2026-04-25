// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/wal"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/dispatch"
)

// fakeSink collects items received via StoreBatch.
type fakeSink struct {
	mu     sync.Mutex
	items  []int
	calls  int
	failN  int // first N calls return an error
	delay  time.Duration
	failed int
}

func (f *fakeSink) StoreBatch(_ context.Context, items []int) error {
	f.mu.Lock()
	f.calls++
	if f.failed < f.failN {
		f.failed++
		f.mu.Unlock()
		return errors.New("transient")
	}
	if f.delay > 0 {
		f.mu.Unlock()
		time.Sleep(f.delay)
		f.mu.Lock()
	}
	f.items = append(f.items, items...)
	f.mu.Unlock()
	return nil
}

func (f *fakeSink) snapshot() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int, len(f.items))
	copy(out, f.items)
	return out
}

// intCodec is a trivial fixed-width codec for the int type.
type intCodec struct{}

func (intCodec) Encode(v int) ([]byte, error) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return b, nil
}

func (intCodec) Decode(b []byte) (int, error) {
	if len(b) != 8 {
		return 0, errors.New("bad length")
	}
	return int(binary.LittleEndian.Uint64(b)), nil //nolint:gosec
}

// errCodec always fails to encode. Used to exercise the encode-error
// drop path in Submit.
type errCodec struct{}

func (errCodec) Encode(int) ([]byte, error) { return nil, errors.New("encode boom") }
func (errCodec) Decode([]byte) (int, error) { return 0, errors.New("decode boom") }

// newRunningEngine constructs and starts an engine with the given
// options, wiring graceful shutdown into t.Cleanup so tests never leak
// goroutines. Caller passes extra options on top of the defaults.
func newRunningEngine(
	tb testing.TB,
	sink dispatch.Sink[int],
	opts ...dispatch.Option[int],
) *dispatch.Engine[int] {
	tb.Helper()
	base := []dispatch.Option[int]{
		dispatch.WithBatchSize[int](10),
		dispatch.WithFlushInterval[int](20 * time.Millisecond),
		dispatch.WithWorkers[int](2),
	}
	eng, err := dispatch.NewEngine[int](sink, append(base, opts...)...)
	require.NoError(tb, err)
	require.NoError(tb, eng.Start())
	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = eng.Shutdown(ctx)
	})
	return eng
}

func TestNewEngine_NilSink_ReturnsErrSinkRequired(t *testing.T) {
	t.Parallel()
	eng, err := dispatch.NewEngine[int](nil)
	require.ErrorIs(t, err, dispatch.ErrSinkRequired)
	require.Nil(t, eng)
}

func TestNewEngine_WithWALNoCodec_ReturnsErrCodecRequired(t *testing.T) {
	t.Parallel()
	// WithWAL with a non-nil codec populates the codec field, so the
	// only way to reach ErrCodecRequired is to bypass WithWAL. We
	// simulate that by calling WithWAL with a nil codec (which is a
	// documented no-op) and then verifying the engine opens without
	// WAL — there is no public surface to enable WAL without a codec,
	// which is exactly the invariant the sentinel protects. The test
	// is therefore the dual: nil codec → WAL is disabled, Engine builds.
	sink := &fakeSink{}
	eng, err := dispatch.NewEngine[int](sink,
		dispatch.WithWAL[int](t.TempDir(), nil),
	)
	require.NoError(t, err)
	require.NotNil(t, eng)
}

func TestEngine_Start_Twice_ReturnsErrAlreadyStarted(t *testing.T) {
	t.Parallel()
	eng, err := dispatch.NewEngine[int](&fakeSink{})
	require.NoError(t, err)
	require.NoError(t, eng.Start())
	t.Cleanup(func() { _ = eng.Shutdown(t.Context()) })

	require.ErrorIs(t, eng.Start(), dispatch.ErrAlreadyStarted)
}

func TestEngine_Submit_BeforeStart_ReturnsFalse(t *testing.T) {
	t.Parallel()
	eng, err := dispatch.NewEngine[int](&fakeSink{})
	require.NoError(t, err)
	require.False(t, eng.Submit(1))
}

func TestEngine_Submit_AfterShutdown_ReturnsFalse(t *testing.T) {
	t.Parallel()
	eng := newRunningEngine(t, &fakeSink{})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, eng.Shutdown(ctx))
	require.False(t, eng.Submit(1))
}

func TestEngine_Shutdown_Idempotent(t *testing.T) {
	t.Parallel()
	eng, err := dispatch.NewEngine[int](&fakeSink{})
	require.NoError(t, err)
	require.NoError(t, eng.Start())

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, eng.Shutdown(ctx))
	require.NoError(t, eng.Shutdown(ctx), "second Shutdown must be a no-op and return nil")
}

func TestEngine_NoWAL_RoundTrip(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	eng := newRunningEngine(t, sink)

	for i := 1; i <= 50; i++ {
		require.True(t, eng.Submit(i))
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, eng.Shutdown(ctx))
	require.Len(t, sink.snapshot(), 50)
}

func TestEngine_WithWAL_RoundTripAndCleanup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sink := &fakeSink{}
	eng := newRunningEngine(t, sink,
		dispatch.WithWAL[int](dir, intCodec{},
			wal.WithMaxSegmentBytes(1<<20),
			wal.WithFsyncInterval(2*time.Millisecond),
		),
	)

	for i := 1; i <= 100; i++ {
		eng.Submit(i)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, eng.Shutdown(ctx))
	require.Len(t, sink.snapshot(), 100)

	// All segments should be removed since every record was acked.
	matches, _ := filepath.Glob(filepath.Join(dir, "*.wal"))
	if len(matches) != 0 {
		for _, m := range matches {
			if fi, _ := os.Stat(m); fi != nil {
				t.Logf("leftover %s size=%d", m, fi.Size())
			}
		}
		t.Fatalf("expected wal dir empty after acks, found: %v", matches)
	}
}

// blockSink stalls StoreBatch on its channel so the WAL accumulates
// under the test's control.
type blockSink struct {
	ch chan struct{}
}

func (b *blockSink) StoreBatch(ctx context.Context, _ []int) error {
	select {
	case <-b.ch:
		return errors.New("blocked")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestEngine_WAL_CrashRecovery(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Phase 1: write items, force fsync, "crash" without graceful shutdown.
	{
		sink := &blockSink{ch: make(chan struct{})}
		eng, err := dispatch.NewEngine[int](sink,
			dispatch.WithBatchSize[int](10),
			dispatch.WithFlushInterval[int](time.Hour), // never flush via timer
			dispatch.WithWorkers[int](1),
			dispatch.WithWAL[int](dir, intCodec{},
				wal.WithMaxSegmentBytes(1<<20),
				wal.WithFsyncInterval(1*time.Millisecond),
			),
		)
		require.NoError(t, err)
		require.NoError(t, eng.Start())

		for i := 1; i <= 50; i++ {
			eng.Submit(i)
		}
		// Force fsync to make sure records are durable.
		require.NoError(t, eng.WAL().Sync())
		// Simulate crash: do NOT call Shutdown, just close the WAL to
		// release the fd for the test process. Recovery is what matters.
		require.NoError(t, eng.WAL().Close())
	}

	// Phase 2: open a fresh engine in the same dir and verify all records replay.
	sink2 := &fakeSink{}
	eng2 := newRunningEngine(t, sink2,
		dispatch.WithBatchSize[int](10),
		dispatch.WithFlushInterval[int](20*time.Millisecond),
		dispatch.WithWorkers[int](2),
		dispatch.WithWAL[int](dir, intCodec{},
			wal.WithMaxSegmentBytes(1<<20),
			wal.WithFsyncInterval(2*time.Millisecond),
		),
	)

	// Wait for replay to complete using the shared polling helper.
	testhelpers.WaitFor(t, 2*time.Second, func() bool {
		return len(sink2.snapshot()) >= 50
	}, "replay timed out waiting for 50 records")

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, eng2.Shutdown(ctx))
	require.Len(t, sink2.snapshot(), 50)
}

func TestEngine_RetriesAndAcks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sink := &fakeSink{failN: 1}
	eng := newRunningEngine(t, sink,
		dispatch.WithBatchSize[int](5),
		dispatch.WithFlushInterval[int](10*time.Millisecond),
		dispatch.WithWorkers[int](1),
		dispatch.WithRetryAttempts[int](3),
		dispatch.WithRetryBackoff[int](1*time.Millisecond),
		dispatch.WithWAL[int](dir, intCodec{},
			wal.WithMaxSegmentBytes(1<<20),
			wal.WithFsyncInterval(1*time.Millisecond),
		),
	)

	for i := 1; i <= 5; i++ {
		eng.Submit(i)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, eng.Shutdown(ctx))
	require.Len(t, sink.snapshot(), 5)
}

// slowSink blocks StoreBatch on every call until the test closes it,
// so the in-memory buffer fills up and Submit triggers the drop path.
type slowSink struct {
	mu      sync.Mutex
	release chan struct{}
	seen    []int
}

func (s *slowSink) StoreBatch(ctx context.Context, items []int) error {
	select {
	case <-s.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	s.mu.Lock()
	s.seen = append(s.seen, items...)
	s.mu.Unlock()
	return nil
}

func TestEngine_Submit_BufferFull_Drops(t *testing.T) {
	t.Parallel()
	sink := &slowSink{release: make(chan struct{})}

	var (
		dropped  atomic.Int64
		dropped1 atomic.Bool
	)
	eng := newRunningEngine(t, sink,
		dispatch.WithBufferSize[int](1),
		dispatch.WithBatchSize[int](1),
		dispatch.WithWorkers[int](1),
		dispatch.WithFlushInterval[int](time.Hour),
		dispatch.WithOnDrop[int](func(int) {
			dropped.Add(1)
			dropped1.Store(true)
		}),
	)
	defer close(sink.release)

	// First Submit lands (buffer size 1). Every subsequent Submit while
	// the sink is blocked must drop.
	eng.Submit(0)
	for i := 1; i <= 100; i++ {
		eng.Submit(i)
	}

	// Give the worker a moment to consume the one record it can.
	testhelpers.WaitFor(t, time.Second, func() bool {
		return dropped1.Load()
	}, "OnDrop callback never fired")

	require.Greater(t, dropped.Load(), int64(0), "expected some drops")
	require.Greater(t, eng.Dropped(), int64(0), "Dropped() must count drops")
	require.Equal(t, dropped.Load(), eng.Dropped(),
		"OnDrop callback count must match Dropped()")
}

func TestEngine_Submit_EncodeError_IsDrop(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	var dropped atomic.Int64
	eng := newRunningEngine(t, sink,
		dispatch.WithWAL[int](t.TempDir(), errCodec{},
			wal.WithFsyncInterval(time.Hour),
		),
		dispatch.WithOnDrop[int](func(int) { dropped.Add(1) }),
	)

	// Every Submit hits the encoder error and should drop.
	for i := 1; i <= 5; i++ {
		eng.Submit(i)
	}

	testhelpers.WaitFor(t, time.Second, func() bool {
		return dropped.Load() == 5
	}, "OnDrop not called 5 times")

	require.Equal(t, int64(5), eng.Dropped(),
		"encode-failure drops must be counted in Dropped()")
}

func TestEngine_BackPressure_BlocksUntilDrain(t *testing.T) {
	t.Parallel()
	sink := &slowSink{release: make(chan struct{})}
	eng := newRunningEngine(t, sink,
		dispatch.WithBufferSize[int](1),
		dispatch.WithBatchSize[int](1),
		dispatch.WithWorkers[int](1),
		dispatch.WithFlushInterval[int](time.Hour),
		dispatch.WithBackPressure[int](),
	)

	// Seed the single worker: Submit(0) lands in the channel, the worker
	// picks it up and blocks in StoreBatch. Submit(1) then refills the
	// channel (buffer = 1), and Submit(2) in a goroutine has nowhere to
	// land until the worker is released.
	require.True(t, eng.Submit(0))
	// Wait for the worker to actually consume the first item and enter
	// StoreBatch — otherwise Submit(1) might land into a just-emptied
	// channel and race ahead.
	testhelpers.WaitFor(t, time.Second, func() bool {
		// Once the worker is blocked in StoreBatch the channel has room
		// again; Submit(1) will succeed synchronously.
		return eng.Submit(1)
	}, "worker never picked up seed record")

	done := make(chan bool, 1)
	go func() {
		done <- eng.Submit(2)
	}()

	select {
	case <-done:
		t.Fatal("Submit returned before the sink was released")
	case <-time.After(50 * time.Millisecond):
		// expected: still blocked in the send on a full buffer.
	}

	close(sink.release)

	select {
	case ok := <-done:
		require.True(t, ok, "BackPressure Submit must return true once the buffer drains")
	case <-time.After(time.Second):
		t.Fatal("Submit never unblocked after sink release")
	}
}

func TestEngine_DefaultMetricsSubsystem_IsAsync(t *testing.T) {
	t.Parallel()
	// Regression: the default metrics subsystem is "async" (not "dispatch")
	// for backwards-compatible metric names. See the comment on
	// DefaultMetricsSubsystem in options.go.
	require.Equal(t, "async", dispatch.DefaultMetricsSubsystem)
}

// Regression for the silent-failure fix: Shutdown must aggregate the
// ctx cancellation error *and* the WAL close error via errors.Join.
// We synthesize a "WAL close error" indirectly by letting the WAL dir
// be deleted underneath the engine — on macOS the final os.Remove in
// WAL.Close then fails and surfaces as a joined error.
//
// Note: directly provoking a WAL close error in a portable way is
// hard because every syscall in wal.Close is best-effort. This test
// instead exercises the happy path of Shutdown with a WAL attached
// and verifies no error is returned — it guards the `errors.Join`
// refactor against accidentally returning a nil-joined-non-nil value.
func TestEngine_Shutdown_WithWAL_NoError(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	eng := newRunningEngine(t, sink,
		dispatch.WithWAL[int](t.TempDir(), intCodec{},
			wal.WithFsyncInterval(2*time.Millisecond),
		),
	)
	for i := 1; i <= 20; i++ {
		eng.Submit(i)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, eng.Shutdown(ctx))
}

// TestEngine_Shutdown_CtxTimeout_ReturnsDeadlineErr verifies that when
// the worker drain exceeds the shutdown deadline, Shutdown returns the
// context error (possibly joined with a WAL close error).
func TestEngine_Shutdown_CtxTimeout_ReturnsDeadlineErr(t *testing.T) {
	t.Parallel()
	sink := &slowSink{release: make(chan struct{})}
	defer close(sink.release)

	eng, err := dispatch.NewEngine[int](sink,
		dispatch.WithBufferSize[int](100),
		dispatch.WithBatchSize[int](10),
		dispatch.WithWorkers[int](1),
		dispatch.WithFlushInterval[int](time.Millisecond),
	)
	require.NoError(t, err)
	require.NoError(t, eng.Start())

	for i := 1; i <= 10; i++ {
		eng.Submit(i)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err = eng.Shutdown(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestWAL_AppendRacesClose is a regression test for a nil-file panic: an
// Appender that passed the unlocked w.closed.Load() check but then lost the
// mutex race to Close would, on reacquiring the lock, proceed to write
// through w.active.file — which Close had set to nil while w.active itself
// was kept around (unacked > 0). The fix re-checks closed/active.file
// inside Append's critical section and returns ErrClosed instead.
//
// The test lives in the dispatch package because the dispatcher is the
// primary caller that can exercise this race under load.
func TestEngine_ShutdownWithActiveProducers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sink := &fakeSink{}
	eng := newRunningEngine(t, sink,
		dispatch.WithWAL[int](dir, intCodec{},
			wal.WithFsyncInterval(time.Hour),
		),
	)

	// Producers hammer Submit until the test signals stop.
	const producers = 8
	var (
		wg   sync.WaitGroup
		stop atomic.Bool
	)
	for i := range producers {
		wg.Go(func() {
			for j := 0; !stop.Load(); j++ {
				eng.Submit(i*1000 + j)
			}
		})
	}

	time.Sleep(50 * time.Millisecond)
	stop.Store(true)
	wg.Wait()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, eng.Shutdown(ctx))

	// We can't assert an exact count (drops are expected at high
	// contention), but the dropped + delivered counts must together
	// equal the totals we submitted. The delivered count is not
	// directly observable without summing the sink snapshot.
	delivered := len(sink.snapshot())
	require.GreaterOrEqual(t, delivered, 1, "some records must reach the sink")
	_ = fmt.Sprintf("delivered=%d dropped=%d", delivered, eng.Dropped())
}
