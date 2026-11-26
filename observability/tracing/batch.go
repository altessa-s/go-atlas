// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
)

const (
	batchWaiting int32 = iota
	batchInflight
	batchDone
)

type flushRequest struct {
	ctx    context.Context
	result chan error
}

type batchProcessor struct {
	adapter   adapters.Adapter
	exporters []adapters.Adapter
	queueSize int
	batchSize int
	timeout   time.Duration
	full      chan struct{}
	flushes   chan flushRequest
	metrics   *exportMetrics

	active        atomic.Pointer[spanBatch]
	mu            sync.Mutex
	spare         []adapters.SpanData
	sealed        []*spanBatch // FIFO protected by mu
	inflight      *spanBatch   // taken batch currently exporting, protected by mu
	waiting       int          // spans in sealed batches, including unfinished writers
	stopping      chan struct{}
	done          chan struct{}
	shutdownCtx   context.Context // published by closing stopping
	cancel        context.CancelFunc
	stopCancel    func() bool
	shutdownErr   error // published by closing done
	lastExportErr error // worker-owned, consumed on flush
}

func flattenAdapters(a adapters.Adapter) []adapters.Adapter {
	if m, ok := a.(*adapters.MultiAdapter); ok {
		if children := m.Adapters(); len(children) > 0 {
			out := make([]adapters.Adapter, 0, len(children))
			for _, child := range children {
				out = append(out, flattenAdapters(child)...)
			}
			return out
		}
	}
	return []adapters.Adapter{a}
}

func newBatchProcessor(cfg *options) *batchProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	exporters := flattenAdapters(cfg.adapter)
	p := &batchProcessor{
		adapter:   cfg.adapter,
		exporters: exporters,
		queueSize: cfg.maxQueueSize,
		batchSize: min(cfg.maxQueueSize, cfg.maxExportBatchSize),
		timeout:   cfg.exportTimeout,
		full:      make(chan struct{}, 1),
		flushes:   make(chan flushRequest),
		stopping:  make(chan struct{}),
		done:      make(chan struct{}),
		cancel:    cancel,
	}
	p.openBatch()
	p.metrics = newExportMetrics(ctx, cfg.meter, exporters, p.queueLength)
	go p.run(ctx, cfg.scheduledDelay)
	return p
}

func (p *batchProcessor) stopped() bool {
	select {
	case <-p.stopping:
		return true
	default:
		return false
	}
}

// Producers reserve distinct slots in the active batch. Sealing prevents new
// reservations; done closes after the last reserved slot has been filled.
type spanBatch struct {
	reserved  atomic.Int64
	remaining atomic.Int64
	state     atomic.Int32
	spans     []adapters.SpanData
	done      chan struct{}
	count     int // set when sealed under the queue lock
}

func (p *batchProcessor) enqueue(span *adapters.SpanData) {
	for {
		b := p.active.Load()
		if b == nil {
			if p.metrics != nil && !p.stopped() {
				p.metrics.pendingDrops.Add(1)
			}
			return
		}
		i := b.reserved.Add(1) - 1
		if i < int64(len(b.spans)) {
			b.spans[i] = *span
			if b.remaining.Add(-1) == 0 {
				close(b.done)
				select {
				case p.full <- struct{}{}:
				default:
				}
			}
			return
		}
		p.mu.Lock()
		if b == p.active.Load() {
			p.seal(b)
			p.openBatch()
		}
		p.mu.Unlock()
	}
}

// openBatch reserves at most the space still available in the bounded queue.
// The worker returns capacity as it hands a batch to the adapter.
func (p *batchProcessor) openBatch() {
	capacity := min(p.batchSize, p.queueSize-p.waiting)
	if capacity == 0 || p.stopped() {
		p.active.Store(nil)
		return
	}
	b := &spanBatch{done: make(chan struct{})}
	if cap(p.spare) >= capacity {
		b.spans = p.spare[:capacity]
		p.spare = nil
	} else {
		b.spans = make([]adapters.SpanData, capacity)
	}
	b.remaining.Store(int64(capacity))
	p.active.Store(b)
}

func (p *batchProcessor) seal(b *spanBatch) {
	capacity := int64(len(b.spans))
	b.count = int(min(b.reserved.Swap(capacity), capacity))
	if unused := len(b.spans) - b.count; unused > 0 && b.remaining.Add(-int64(unused)) == 0 {
		close(b.done)
	}
	if b.count > 0 {
		p.sealed = append(p.sealed, b)
		p.waiting += b.count
	}
}

func (p *batchProcessor) queueLength() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := p.waiting
	if b := p.active.Load(); b != nil {
		n += int(min(b.reserved.Load(), int64(len(b.spans))))
	}
	return n
}

// snapshot bounds a flush to batches admitted before it took the queue lock.
func (p *batchProcessor) snapshot(includePartial bool) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if b := p.active.Load(); b != nil {
		n := b.reserved.Load()
		if n >= int64(len(b.spans)) || (includePartial && n > 0) {
			p.seal(b)
			p.openBatch()
		}
	}
	return len(p.sealed)
}

func (p *batchProcessor) take(ctx context.Context) (*spanBatch, []adapters.SpanData) {
	p.mu.Lock()
	if len(p.sealed) == 0 {
		p.mu.Unlock()
		return nil, nil
	}
	b := p.sealed[0]
	p.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, nil
	case <-b.done:
	}

	spans := b.spans[:b.count:b.count]
	// A timer or explicit flush may cut a mostly empty batch. Keep one
	// cleared buffer and give the adapter only the occupied slots.
	var spare []adapters.SpanData
	if b.count < len(b.spans)/2 {
		spans = slices.Clone(spans)
		clear(b.spans[:b.count])
		spare = b.spans
	}
	p.mu.Lock()
	if len(p.sealed) == 0 || p.sealed[0] != b || !b.state.CompareAndSwap(batchWaiting, batchInflight) {
		p.mu.Unlock()
		return nil, nil
	}
	if cap(spare) > cap(p.spare) {
		p.spare = spare
	}
	p.sealed[0] = nil
	p.sealed = p.sealed[1:]
	p.waiting -= b.count
	p.inflight = b
	if p.active.Load() == nil {
		p.openBatch()
	}
	p.mu.Unlock()
	return b, spans
}

func (p *batchProcessor) drainUnfinished() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	var dropped uint64
	if b := p.inflight; b != nil {
		if b.state.CompareAndSwap(batchInflight, batchDone) {
			dropped += uint64(b.count)
		}
		p.inflight = nil
	}
	for _, b := range p.sealed {
		if b.state.CompareAndSwap(batchWaiting, batchDone) {
			dropped += uint64(b.count)
		}
	}
	p.sealed = nil
	p.spare = nil
	p.waiting = 0
	return dropped
}

func (p *batchProcessor) abandonRemainingAndFence() {
	if p.metrics != nil {
		p.metrics.abandonAndDisable(p.drainUnfinished)
		return
	}
	p.drainUnfinished()
}

func (p *batchProcessor) forceFlush(ctx context.Context) error {
	if p.stopped() {
		return ErrTracerShutdown
	}
	req := flushRequest{ctx: ctx, result: make(chan error, 1)}
	select {
	case <-p.stopping:
		return ErrTracerShutdown
	case <-ctx.Done():
		return ctx.Err()
	case p.flushes <- req:
	}
	select {
	case err := <-req.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *batchProcessor) shutdown(ctx context.Context) error {
	select {
	case <-p.done:
		if p.metrics != nil {
			p.metrics.disable()
		}
		return p.shutdownErr
	default:
	}
	p.mu.Lock()
	if !p.stopped() {
		p.shutdownCtx = ctx
		// Interrupt even an export that started before Shutdown.
		p.stopCancel = context.AfterFunc(ctx, p.cancel)
		close(p.stopping)
		if b := p.active.Load(); b != nil {
			p.seal(b)
			p.active.Store(nil)
		}
	}
	shutdownCtx := p.shutdownCtx
	p.mu.Unlock()
	select {
	case <-p.done:
		p.abandonRemainingAndFence()
		return p.shutdownErr
	case <-ctx.Done():
		if shutdownCtx != nil && shutdownCtx.Err() != nil {
			p.abandonRemainingAndFence()
		} else if p.metrics != nil {
			p.metrics.disable()
		}
		return ctx.Err()
	}
}

//nolint:contextcheck // Explicit flush and shutdown requests carry their caller's context into the worker.
func (p *batchProcessor) run(ctx context.Context, delay time.Duration) {
	defer close(p.done)
	ticker := time.NewTicker(delay)
	defer ticker.Stop()
	for !p.stopped() {
		select {
		case <-p.stopping:
		case req := <-p.flushes:
			flushCtx, cancel := context.WithCancel(req.ctx)
			stop := context.AfterFunc(ctx, cancel)
			req.result <- p.flush(flushCtx)
			stop()
			cancel()
		case <-ticker.C:
			p.export(ctx, p.snapshot(true))
		case <-p.full:
			p.export(ctx, p.snapshot(false))
		}
	}

	p.mu.Lock()
	shutdownCtx := p.shutdownCtx
	p.mu.Unlock()

	flushErr := p.flush(shutdownCtx)
	p.shutdownErr = errors.Join(flushErr, p.adapter.Shutdown(shutdownCtx))
	p.cancel()
	if p.metrics != nil {
		<-p.metrics.done
	}
	p.abandonRemainingAndFence()
	p.stopCancel()
	p.stopCancel = nil
}

func (p *batchProcessor) flush(ctx context.Context) error {
	// Only the worker dequeues. A snapshot covers all prior enqueues without
	// extending the flush indefinitely when producers keep adding spans.
	p.export(ctx, p.snapshot(true))
	err := p.lastExportErr
	p.lastExportErr = nil
	if p.metrics != nil && ctx.Err() == nil {
		p.metrics.sample()
	}
	if ctx.Err() != nil {
		return errors.Join(err, ctx.Err())
	}
	return errors.Join(err, p.adapter.ForceFlush(ctx))
}

func (p *batchProcessor) export(ctx context.Context, count int) {
	for count > 0 && ctx.Err() == nil {
		b, batch := p.take(ctx)
		if b == nil {
			return
		}
		exportCtx, cancel := context.WithTimeout(ctx, p.timeout)
		outcomes, acceptedByAll, err := p.exportToExporters(exportCtx, batch)
		cancel()

		if err != nil {
			p.lastExportErr = err
			p.mu.Lock()
			shutdownExpired := p.shutdownCtx != nil && p.shutdownCtx.Err() != nil
			p.mu.Unlock()
			if shutdownExpired {
				// Leave b in batchInflight so shutdown deadline abandonment
				// accounts for its spans in spans_dropped_total.
				return
			}
		}

		if p.metrics != nil {
			p.metrics.completeBatch(&b.state, acceptedByAll, outcomes)
		} else {
			b.state.CompareAndSwap(batchInflight, batchDone)
		}
		p.mu.Lock()
		if p.inflight == b {
			p.inflight = nil
		}
		p.mu.Unlock()

		count--
	}
}

func (p *batchProcessor) exportToExporters(ctx context.Context, batch []adapters.SpanData) ([]exporterOutcome, int, error) {
	outcomes := make([]exporterOutcome, len(p.exporters))
	var errs []error
	var maxUnaccepted int
	for i, exp := range p.exporters {
		res, err := exp.ExportSpans(ctx, batch)
		if err != nil {
			errs = append(errs, err)
			outcomes[i] = exporterOutcome{failed: true}
			maxUnaccepted = len(batch)
			continue
		}
		rej := min(max(res.Rejected, 0), len(batch))
		outcomes[i] = exporterOutcome{rejected: rej}
		if rej > maxUnaccepted {
			maxUnaccepted = rej
		}
	}
	return outcomes, len(batch) - maxUnaccepted, errors.Join(errs...)
}
