// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
)

type batchAdapter struct {
	*adapters.MultiAdapter
	export func(context.Context, []adapters.SpanData) error
}

func (a batchAdapter) ExportSpans(ctx context.Context, spans []adapters.SpanData) (adapters.ExportResult, error) {
	if err := a.export(ctx, spans); err != nil {
		return adapters.ExportResult{}, err
	}
	return adapters.ExportResult{Accepted: len(spans)}, nil
}

func endSpan(rec tracing.Recorder, ctx context.Context) {
	_, span := rec.Start(ctx, "statement")
	span.End()
}

func TestBatchTriggersAndDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var batches []int
		a := batchAdapter{adapters.NewMultiAdapter(), func(ctx context.Context, spans []adapters.SpanData) error {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) != 10*time.Second {
				t.Errorf("unexpected export deadline %v", deadline)
			}
			batches = append(batches, len(spans))
			return nil
		}}
		tr := tracing.New(tracing.WithAdapter(a), tracing.WithMaxExportBatchSize(2))
		rec := tr.Recorder("test")
		endSpan(rec, t.Context())
		synctest.Wait()
		require.Empty(t, batches)
		endSpan(rec, t.Context())
		synctest.Wait()
		require.Equal(t, []int{2}, batches)
		endSpan(rec, t.Context())
		time.Sleep(time.Second)
		synctest.Wait()
		require.Equal(t, []int{2, 1}, batches)
		_, late := rec.Start(t.Context(), "late")
		endSpan(rec, t.Context())
		require.NoError(t, tr.Shutdown(t.Context()))
		late.End()
		require.Equal(t, []int{2, 1, 1}, batches)
		require.ErrorIs(t, tr.ForceFlush(t.Context()), tracing.ErrTracerShutdown)
	})
}

func TestBlockedExport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		a := batchAdapter{adapters.NewMultiAdapter(), func(ctx context.Context, _ []adapters.SpanData) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}}
		meter := metricstest.NewTestCollector()
		tr := tracing.New(tracing.WithAdapter(a), tracing.WithMaxExportBatchSize(1), tracing.WithMaxQueueSize(8),
			tracing.WithMeter(meter.Collector), tracing.WithExportTimeout(time.Hour))
		rec := tr.Recorder("test")
		endSpan(rec, t.Context())
		<-started
		var ends sync.WaitGroup
		for range 64 {
			ends.Go(func() { endSpan(rec, t.Context()) })
		}
		ends.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		require.Equal(t, 8.0, meter.GaugeValue("test_tracing_queue_length", nil))
		require.Equal(t, 56.0, meter.CounterValue("test_tracing_spans_dropped_total", nil))
		ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
		defer cancel()
		require.ErrorIs(t, tr.ForceFlush(ctx), context.DeadlineExceeded)
		ctx, stop := context.WithTimeout(t.Context(), time.Millisecond)
		defer stop()
		require.ErrorIs(t, tr.Shutdown(ctx), context.DeadlineExceeded)
		synctest.Wait()
		require.Equal(t, 0.0, meter.GaugeValue("test_tracing_queue_length", nil))
		require.Zero(t, meter.CounterValue("test_tracing_export_failures_total", nil))
	})
}

func TestForceFlushCompletesPriorSpansOnce(t *testing.T) {
	const count = 100
	meter := metricstest.NewTestCollector()
	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithMeter(meter.Collector))
	rec := tr.Recorder("test")
	for range count {
		_, span := rec.Start(t.Context(), "statement")
		var ends sync.WaitGroup
		for range 4 {
			ends.Go(func() { span.End() })
		}
		ends.Wait()
	}
	var flushes sync.WaitGroup
	for range 4 {
		flushes.Go(func() {
			if err := tr.ForceFlush(t.Context()); err != nil {
				t.Errorf("ForceFlush failed: %v", err)
			}
		})
	}
	flushes.Wait()
	require.Len(t, a.spans, count)
	ids := make(map[adapters.SpanID]bool)
	for i := range a.spans {
		ids[a.spans[i].SpanID] = true
	}
	require.Len(t, ids, count)
	require.Equal(t, float64(count), meter.CounterValue("test_tracing_spans_exported_total", nil))
	require.NoError(t, tr.Shutdown(t.Context()))
}

func TestExportTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := batchAdapter{adapters.NewMultiAdapter(), func(ctx context.Context, _ []adapters.SpanData) error {
			<-ctx.Done()
			return ctx.Err()
		}}
		tr := tracing.New(tracing.WithAdapter(a), tracing.WithExportTimeout(time.Millisecond))
		endSpan(tr.Recorder("test"), t.Context())
		start := time.Now()
		require.ErrorIs(t, tr.ForceFlush(t.Context()), context.DeadlineExceeded)
		require.Equal(t, time.Millisecond, time.Since(start))
		require.NoError(t, tr.Shutdown(t.Context()))
	})
}

func TestShutdownDeadlineWithUnresponsiveAdapter(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		started := make(chan struct{})
		a := batchAdapter{adapters.NewMultiAdapter(), func(context.Context, []adapters.SpanData) error {
			close(started)
			<-release
			return nil
		}}
		meter := metricstest.NewTestCollector()
		tr := tracing.New(tracing.WithAdapter(a), tracing.WithMaxExportBatchSize(1), tracing.WithMeter(meter))
		rec := tr.Recorder("test")
		endSpan(rec, t.Context())
		<-started
		endSpan(rec, t.Context())
		ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
		defer cancel()
		require.ErrorIs(t, tr.Shutdown(ctx), context.DeadlineExceeded)
		_, late := rec.Start(t.Context(), "after shutdown")
		require.False(t, late.IsRecording())
		close(release)
		require.ErrorIs(t, tr.Shutdown(t.Context()), context.DeadlineExceeded)
		require.Zero(t, meter.CounterValue("test_tracing_spans_exported_total", nil),
			"an exporter that returns after Shutdown must not record metrics")
	})
}

func TestConcurrentShutdownDeadlineFencesMetrics(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		release, started := make(chan struct{}), make(chan struct{})
		a := batchAdapter{adapters.NewMultiAdapter(), func(context.Context, []adapters.SpanData) error {
			close(started)
			<-release
			return nil
		}}
		meter := metricstest.NewTestCollector()
		tr := tracing.New(tracing.WithAdapter(a), tracing.WithMaxExportBatchSize(1), tracing.WithMeter(meter))
		endSpan(tr.Recorder("test"), t.Context())
		<-started
		first := make(chan error, 1)
		go func() { first <- tr.Shutdown(t.Context()) }()
		synctest.Wait()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := tr.Shutdown(ctx)
		close(release)
		require.ErrorIs(t, err, context.Canceled)
		require.NoError(t, <-first)
		require.Zero(t, meter.CounterValue("test_tracing_spans_exported_total", nil))
	})
}

func BenchmarkStartEndBlockedExporter(b *testing.B) {
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	a := batchAdapter{adapters.NewMultiAdapter(), func(context.Context, []adapters.SpanData) error {
		once.Do(func() { close(started) })
		<-release
		return nil
	}}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithMaxQueueSize(1))
	rec := tr.Recorder("bench")
	ctx := b.Context()
	endSpan(rec, ctx)
	<-started
	endSpan(rec, ctx)
	b.Run("serial", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			endSpan(rec, ctx)
		}
	})
	b.Run("parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				endSpan(rec, ctx)
			}
		})
	})
	close(release)
	require.NoError(b, tr.Shutdown(ctx))
}

func TestBatchSnapshotsAttributes(t *testing.T) {
	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithScheduledDelay(time.Hour))
	rec := tr.Recorder("test")
	ctx, parent := rec.Start(t.Context(), "parent")
	values := []string{"before"}
	attr := tracing.Attribute{Key: "labels", Value: values}
	link := tracing.Link{SpanContext: parent.SpanContext(), Attributes: []tracing.Attribute{attr}}
	_, child := rec.Start(ctx, "child", tracing.WithAttributes(attr), tracing.WithLinks(link))
	child.AddEvent("event", tracing.WithEventAttributes(attr))
	child.End()
	values[0] = "after"
	parent.End()
	require.NoError(t, tr.Shutdown(t.Context()))
	require.Len(t, a.spans, 2)
	childData := a.spans[0]
	want := adapters.Attribute{Key: "labels", Value: []string{"before"}}
	require.Contains(t, childData.Attributes, want)
	require.Contains(t, childData.Events[0].Attributes, want)
	require.Contains(t, childData.Links[0].Attributes, want)
}

func TestBatchPreservesAdmissionOrderAndCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const batchSize, capacity = 3, 7
		started, resume := make(chan struct{}), make(chan struct{})
		var retained [][]adapters.SpanData
		a := batchAdapter{adapters.NewMultiAdapter(), func(_ context.Context, spans []adapters.SpanData) error {
			if len(retained) == 0 {
				close(started)
				<-resume
			}
			retained = append(retained, spans)
			return nil
		}}
		meter := metricstest.NewTestCollector()
		tr := tracing.New(tracing.WithAdapter(a), tracing.WithMeter(meter), tracing.WithMaxQueueSize(capacity),
			tracing.WithMaxExportBatchSize(batchSize), tracing.WithScheduledDelay(time.Hour))
		rec := tr.Recorder("test")
		end := func(name string) {
			_, span := rec.Start(t.Context(), name)
			span.End()
		}
		names := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
		for _, name := range names[:batchSize] {
			end(name)
		}
		<-started

		flush := make(chan error, 1)
		go func() { flush <- tr.ForceFlush(t.Context()) }()
		synctest.Wait()
		for _, name := range names[batchSize:] {
			end(name)
		}
		end("full")
		time.Sleep(time.Second)
		synctest.Wait()
		require.EqualValues(t, capacity, meter.GaugeValue("test_tracing_queue_length", nil))
		require.EqualValues(t, 1, meter.CounterValue("test_tracing_spans_dropped_total", nil))
		stopped := make(chan error, 1)
		go func() { stopped <- tr.Shutdown(t.Context()) }()
		synctest.Wait()
		end("closed")
		close(resume)
		require.NoError(t, <-stopped)
		require.ErrorIs(t, <-flush, tracing.ErrTracerShutdown)

		var got []string
		for _, batch := range retained {
			require.LessOrEqual(t, len(batch), batchSize)
			for _, span := range batch {
				got = append(got, span.Name)
			}
		}
		require.Equal(t, names, got)
		require.Zero(t, meter.GaugeValue("test_tracing_queue_length", nil))
	})
}

func TestForceFlushRetainsItsAdmissionPrefix(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var retained [][]adapters.SpanData
		started, release := make(chan struct{}), make(chan struct{})
		a := batchAdapter{adapters.NewMultiAdapter(), func(_ context.Context, spans []adapters.SpanData) error {
			retained = append(retained, spans)
			if len(retained) == 1 {
				close(started)
				<-release
			}
			return nil
		}}
		tr := tracing.New(tracing.WithAdapter(a), tracing.WithScheduledDelay(time.Hour))
		rec := tr.Recorder("test")
		end := func(name string) {
			_, span := rec.Start(t.Context(), name)
			span.End()
		}
		end("first")
		flushed := make(chan error, 1)
		go func() { flushed <- tr.ForceFlush(t.Context()) }()
		<-started
		end("second")
		close(release)
		require.NoError(t, <-flushed)
		require.Len(t, retained, 1)
		require.NoError(t, tr.ForceFlush(t.Context()))
		end("third")
		require.NoError(t, tr.Shutdown(t.Context()))
		require.Len(t, retained, 3)
		for i, name := range []string{"first", "second", "third"} {
			require.Len(t, retained[i], 1)
			require.Equal(t, name, retained[i][0].Name)
		}
	})
}

func TestCompletedShutdownWithCanceledContext(t *testing.T) {
	tr := tracing.New(tracing.WithAdapter(adapters.NewMultiAdapter()))
	require.NoError(t, tr.Shutdown(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Both the worker and the caller are done. A repeated shutdown must
	// consistently return the completed operation's result.
	for range 100 {
		require.NoError(t, tr.Shutdown(ctx))
	}
}

func TestShutdownDeadlineCountsDroppedSpans(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		const total = 100
		started := make(chan struct{})
		var once sync.Once
		a := batchAdapter{adapters.NewMultiAdapter(), func(ctx context.Context, _ []adapters.SpanData) error {
			once.Do(func() { close(started) })
			<-ctx.Done()
			return ctx.Err()
		}}
		meter := metricstest.NewTestCollector()
		tr := tracing.New(
			tracing.WithAdapter(a),
			tracing.WithMaxExportBatchSize(10),
			tracing.WithMaxQueueSize(200),
			tracing.WithScheduledDelay(time.Hour),
			tracing.WithExportTimeout(time.Hour),
			tracing.WithMeter(meter),
		)
		rec := tr.Recorder("test")
		for range total {
			endSpan(rec, t.Context())
		}
		<-started

		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()
		require.ErrorIs(t, tr.Shutdown(ctx), context.DeadlineExceeded)
		synctest.Wait()

		exported := meter.CounterValue("test_tracing_spans_exported_total", nil)
		dropped := meter.CounterValue("test_tracing_spans_dropped_total", nil)
		require.Equal(t, float64(total)-exported, dropped)
	})
}

func TestShutdownSecondaryCallerDeadlinePreservesDropAccounting(t *testing.T) {
	t.Parallel()

	t.Run("blocked_exporter_counts_all_drops", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			const total = 100
			started := make(chan struct{})
			var once sync.Once
			a := batchAdapter{adapters.NewMultiAdapter(), func(ctx context.Context, _ []adapters.SpanData) error {
				once.Do(func() { close(started) })
				<-ctx.Done()
				return ctx.Err()
			}}
			meter := metricstest.NewTestCollector()
			tr := tracing.New(
				tracing.WithAdapter(a),
				tracing.WithMaxExportBatchSize(10),
				tracing.WithMaxQueueSize(200),
				tracing.WithScheduledDelay(time.Hour),
				tracing.WithExportTimeout(time.Hour),
				tracing.WithMeter(meter),
			)
			rec := tr.Recorder("test")
			for range total {
				endSpan(rec, t.Context())
			}
			<-started

			primaryCtx, primaryCancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
			defer primaryCancel()
			primaryDone := make(chan error, 1)
			go func() { primaryDone <- tr.Shutdown(primaryCtx) }()
			synctest.Wait()

			secondaryCtx, secondaryCancel := context.WithTimeout(t.Context(), time.Millisecond)
			defer secondaryCancel()
			require.ErrorIs(t, tr.Shutdown(secondaryCtx), context.DeadlineExceeded)
			require.ErrorIs(t, <-primaryDone, context.DeadlineExceeded)
			synctest.Wait()

			require.Zero(t, meter.CounterValue("test_tracing_spans_exported_total", nil))
			require.Equal(t, float64(total), meter.CounterValue("test_tracing_spans_dropped_total", nil))
			require.Zero(t, meter.GaugeValue("test_tracing_queue_length", nil))
		})
	})

	t.Run("secondary_cancel_still_delivers_queued_batches_for_primary", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			var mu sync.Mutex
			var exportedSpans int
			a := batchAdapter{adapters.NewMultiAdapter(), func(_ context.Context, spans []adapters.SpanData) error {
				once.Do(func() {
					close(started)
					<-release
				})
				mu.Lock()
				exportedSpans += len(spans)
				mu.Unlock()
				return nil
			}}
			meter := metricstest.NewTestCollector()
			tr := tracing.New(
				tracing.WithAdapter(a),
				tracing.WithMaxExportBatchSize(1),
				tracing.WithMaxQueueSize(10),
				tracing.WithScheduledDelay(time.Hour),
				tracing.WithMeter(meter),
			)
			rec := tr.Recorder("test")
			endSpan(rec, t.Context())
			<-started
			endSpan(rec, t.Context())

			primaryDone := make(chan error, 1)
			go func() { primaryDone <- tr.Shutdown(t.Context()) }()
			synctest.Wait()

			secondaryCtx, secondaryCancel := context.WithCancel(t.Context())
			secondaryCancel()
			require.ErrorIs(t, tr.Shutdown(secondaryCtx), context.Canceled)

			close(release)
			require.NoError(t, <-primaryDone)

			mu.Lock()
			got := exportedSpans
			mu.Unlock()
			require.Equal(t, 2, got)
			require.Zero(t, meter.CounterValue("test_tracing_spans_exported_total", nil))
			require.Equal(t, 2.0, meter.CounterValue("test_tracing_spans_dropped_total", nil))
		})
	})
}

func TestShutdownDeadlineGateNoDoubleCount(t *testing.T) {
	t.Parallel()
	const (
		total     = 100
		batchSize = 25
	)

	t.Run("completion_before_deadline", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			started, gate := make(chan struct{}), make(chan struct{})
			secondBatch := make(chan struct{})
			var calls int
			a := batchAdapter{adapters.NewMultiAdapter(), func(ctx context.Context, _ []adapters.SpanData) error {
				calls++
				switch calls {
				case 1:
					close(started)
					<-gate
					return nil
				case 2:
					close(secondBatch)
					<-ctx.Done()
					return ctx.Err()
				default:
					<-ctx.Done()
					return ctx.Err()
				}
			}}
			meter := metricstest.NewTestCollector()
			tr := tracing.New(
				tracing.WithAdapter(a),
				tracing.WithMaxExportBatchSize(batchSize),
				tracing.WithMaxQueueSize(200),
				tracing.WithScheduledDelay(time.Hour),
				tracing.WithExportTimeout(time.Hour),
				tracing.WithMeter(meter),
			)
			rec := tr.Recorder("test")
			for range total {
				endSpan(rec, t.Context())
			}
			<-started

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- tr.Shutdown(ctx) }()
			synctest.Wait()

			close(gate)
			<-secondBatch
			require.ErrorIs(t, <-done, context.DeadlineExceeded)
			synctest.Wait()

			require.Equal(t, float64(batchSize), meter.CounterValue("test_tracing_spans_exported_total", nil))
			require.Equal(t, float64(total-batchSize), meter.CounterValue("test_tracing_spans_dropped_total", nil))
		})
	})

	t.Run("deadline_before_completion", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			started, gate := make(chan struct{}), make(chan struct{})
			var once sync.Once
			a := batchAdapter{adapters.NewMultiAdapter(), func(ctx context.Context, _ []adapters.SpanData) error {
				isFirst := false
				once.Do(func() {
					isFirst = true
					close(started)
				})
				if isFirst {
					<-gate
					return nil
				}
				<-ctx.Done()
				return ctx.Err()
			}}
			meter := metricstest.NewTestCollector()
			tr := tracing.New(
				tracing.WithAdapter(a),
				tracing.WithMaxExportBatchSize(batchSize),
				tracing.WithMaxQueueSize(200),
				tracing.WithScheduledDelay(time.Hour),
				tracing.WithExportTimeout(time.Hour),
				tracing.WithMeter(meter),
			)
			rec := tr.Recorder("test")
			for range total {
				endSpan(rec, t.Context())
			}
			<-started

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
			defer cancel()
			require.ErrorIs(t, tr.Shutdown(ctx), context.DeadlineExceeded)
			close(gate)
			synctest.Wait()

			require.Zero(t, meter.CounterValue("test_tracing_spans_exported_total", nil))
			require.Equal(t, float64(total), meter.CounterValue("test_tracing_spans_dropped_total", nil))
		})
	})

	t.Run("concurrent_at_deadline", func(t *testing.T) {
		t.Parallel()
		for range 20 {
			synctest.Test(t, func(t *testing.T) {
				started := make(chan struct{})
				gate := make(chan struct{})
				var once sync.Once
				a := batchAdapter{adapters.NewMultiAdapter(), func(ctx context.Context, _ []adapters.SpanData) error {
					isFirst := false
					once.Do(func() {
						isFirst = true
						close(started)
					})
					if isFirst {
						<-gate
						return nil
					}
					<-ctx.Done()
					return ctx.Err()
				}}
				meter := metricstest.NewTestCollector()
				tr := tracing.New(
					tracing.WithAdapter(a),
					tracing.WithMaxExportBatchSize(batchSize),
					tracing.WithMaxQueueSize(200),
					tracing.WithScheduledDelay(time.Hour),
					tracing.WithExportTimeout(time.Hour),
					tracing.WithMeter(meter),
				)
				rec := tr.Recorder("test")
				for range total {
					endSpan(rec, t.Context())
				}
				<-started

				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
				defer cancel()
				stopGate := context.AfterFunc(ctx, func() { close(gate) })
				defer stopGate()

				_ = tr.Shutdown(ctx)
				synctest.Wait()

				exported := meter.CounterValue("test_tracing_spans_exported_total", nil)
				dropped := meter.CounterValue("test_tracing_spans_dropped_total", nil)
				require.Equal(t, float64(total), exported+dropped)
				require.Contains(t, []float64{0, batchSize}, exported)
			})
		}
	})
}

type oneLineThenFailWriter struct {
	sawLine bool
	err     error
}

func (w *oneLineThenFailWriter) Write(p []byte) (int, error) {
	if w.sawLine {
		return 0, w.err
	}
	if bytes.IndexByte(p, '\n') >= 0 {
		w.sawLine = true
	}
	return len(p), nil
}

func TestBatchPartialAcceptanceOnExporterError(t *testing.T) {
	t.Parallel()

	t.Run("single_console_exporter", func(t *testing.T) {
		t.Parallel()
		errWrite := errors.New("disk full")
		exp := console.New(
			console.WithWriter(&oneLineThenFailWriter{err: errWrite}),
			console.WithPrettyPrint(false),
		)
		meter := metricstest.NewTestCollector()
		tr := tracing.New(
			tracing.WithAdapter(exp),
			tracing.WithMeter(meter),
			tracing.WithScheduledDelay(time.Hour),
		)
		defer func() { require.NoError(t, tr.Shutdown(t.Context())) }()

		rec := tr.Recorder("svc")
		endSpan(rec, t.Context())
		endSpan(rec, t.Context())

		require.ErrorIs(t, tr.ForceFlush(t.Context()), errWrite)
		require.Equal(t, 1.0, meter.CounterValue("test_tracing_spans_exported_total", nil))
		require.Equal(t, 1.0, metricstest.GetCounterValue(t, meter, "test_tracing_export_failures_total", "exporter", "console"))
	})

	t.Run("multi_exporter_with_healthy_and_partial_failure", func(t *testing.T) {
		t.Parallel()
		errWrite := errors.New("disk full")
		exp := console.New(
			console.WithWriter(&oneLineThenFailWriter{err: errWrite}),
			console.WithPrettyPrint(false),
		)
		healthy := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
		meter := metricstest.NewTestCollector()
		tr := tracing.New(
			tracing.WithAdapter(adapters.NewMultiAdapter(healthy, exp)),
			tracing.WithMeter(meter),
			tracing.WithScheduledDelay(time.Hour),
		)
		defer func() { require.NoError(t, tr.Shutdown(t.Context())) }()

		rec := tr.Recorder("svc")
		endSpan(rec, t.Context())
		endSpan(rec, t.Context())

		require.ErrorIs(t, tr.ForceFlush(t.Context()), errWrite)
		require.Equal(t, 1.0, meter.CounterValue("test_tracing_spans_exported_total", nil))
		require.Equal(t, 1.0, metricstest.GetCounterValue(t, meter, "test_tracing_export_failures_total", "exporter", "console"))
		require.Equal(t, 0.0, metricstest.GetCounterValue(t, meter, "test_tracing_export_failures_total", "exporter", "multi"))
	})
}
