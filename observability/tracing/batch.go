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

type flushRequest struct {
	ctx    context.Context
	result chan error
}

type batchProcessor struct {
	adapter   adapters.Adapter
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
	waiting       int          // spans in sealed batches, including unfinished writers
	stopping      chan struct{}
	done          chan struct{}
	shutdownCtx   context.Context // published by closing stopping
	cancel        context.CancelFunc
	stopCancel    func() bool
	shutdownErr   error // published by closing done
	lastExportErr error // worker-owned, consumed on flush
}

func newBatchProcessor(cfg *options) *batchProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	p := &batchProcessor{
		adapter:   cfg.adapter,
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
	p.metrics = newExportMetrics(ctx, cfg.meter, p.queueLength)
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

func (p *batchProcessor) take(ctx context.Context) []adapters.SpanData {
	p.mu.Lock()
	b := p.sealed[0]
	p.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil
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
	if cap(spare) > cap(p.spare) {
		p.spare = spare
	}
	p.sealed[0] = nil
	p.sealed = p.sealed[1:]
	p.waiting -= b.count
	if p.active.Load() == nil {
		p.openBatch()
	}
	p.mu.Unlock()
	return spans
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
	if p.metrics != nil {
		defer p.metrics.disable()
	}
	select {
	case <-p.done:
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
	p.mu.Unlock()
	select {
	case <-p.done:
		return p.shutdownErr
	case <-ctx.Done():
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

	flushErr := p.flush(p.shutdownCtx)
	p.shutdownErr = errors.Join(flushErr, p.adapter.Shutdown(p.shutdownCtx))
	// A shutdown deadline can expire with spans still in the queue.
	p.mu.Lock()
	p.sealed = nil
	p.spare = nil
	p.waiting = 0
	p.mu.Unlock()
	p.cancel()
	if p.metrics != nil {
		<-p.metrics.done
		p.metrics.sample(p.shutdownCtx)
	}
	p.stopCancel()
	p.stopCancel = nil
	p.shutdownCtx = nil
}

func (p *batchProcessor) flush(ctx context.Context) error {
	// Only the worker dequeues. A snapshot covers all prior enqueues without
	// extending the flush indefinitely when producers keep adding spans.
	p.export(ctx, p.snapshot(true))
	err := p.lastExportErr
	p.lastExportErr = nil
	if p.metrics != nil {
		p.metrics.sample(ctx)
	}
	if ctx.Err() != nil {
		return errors.Join(err, ctx.Err())
	}
	return errors.Join(err, p.adapter.ForceFlush(ctx))
}

func (p *batchProcessor) export(ctx context.Context, count int) {
	for count > 0 && ctx.Err() == nil {
		batch := p.take(ctx)
		if batch == nil {
			return
		}
		exportCtx, cancel := context.WithTimeout(ctx, p.timeout)
		err := p.adapter.ExportSpans(exportCtx, batch)
		cancel()
		if err != nil {
			p.lastExportErr = err
		}
		if p.metrics != nil {
			p.metrics.recordExport(ctx, len(batch), err)
		}
		count--
	}
}
