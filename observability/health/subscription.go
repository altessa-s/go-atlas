// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

// Subscription is the concrete [Subscriber] returned by [Coordinator.Subscribe].
// Always call [Subscription.Close] when done to release resources.
// Close is safe to call multiple times.
type Subscription struct {
	initialStatus ServingStatus
	updates       <-chan ServingStatus

	watcher *watcher
}

// InitialStatus returns the [ServingStatus] captured at subscription time.
func (s *Subscription) InitialStatus() ServingStatus {
	return s.initialStatus
}

// Updates returns a read-only channel receiving [ServingStatus] changes.
// The channel is closed when [Subscription.Close] is called.
func (s *Subscription) Updates() <-chan ServingStatus {
	return s.updates
}

// Close terminates the subscription and releases resources.
func (s *Subscription) Close() {
	if s.watcher != nil {
		s.watcher.stop(false)
	}
}

type watcherShard struct {
	mu       sync.RWMutex
	watchers map[string]map[*watcher]struct{}
}

type watcher struct {
	ch chan ServingStatus

	stopOnce sync.Once
	remove   func(terminal bool)

	// A poll snapshots pushes before checking the service. Delivery and push
	// sequence changes share mu so a newer push always wins.
	pushes     atomic.Uint64
	mu         sync.Mutex
	lastStatus ServingStatus
	stopped    bool
}

func newWatcher(buffer int, initialStatus ServingStatus) *watcher {
	return &watcher{ch: make(chan ServingStatus, buffer), lastStatus: initialStatus}
}

// notify pushes a status. Even a full buffer supersedes an in-flight poll.
// A subsequent cycle retries a dropped change against lastStatus.
func (w *watcher) notify(status ServingStatus) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pushes.Add(1)
	return w.send(status)
}

func (w *watcher) notifyPoll(status ServingStatus, pushes uint64) (previous ServingStatus, delivered bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	previous = w.lastStatus
	return previous, w.pushes.Load() == pushes && previous != status && w.send(status)
}

// send attempts delivery while holding mu.
func (w *watcher) send(status ServingStatus) bool {
	if w.stopped {
		return false
	}
	select {
	case w.ch <- status:
		w.lastStatus = status
		return true
	default:
		return false
	}
}

func (w *watcher) stop(terminal bool) {
	w.stopOnce.Do(func() { w.remove(terminal) })
}

func (w *watcher) close(terminal bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if terminal {
		// Make room for the terminal update even when the buffer is full.
		if len(w.ch) == cap(w.ch) {
			select {
			case <-w.ch:
			default:
			}
		}
		w.ch <- StatusNotServing
	}
	w.stopped = true
	close(w.ch)
}

type subscriptions struct {
	watcherShards            []watcherShard
	activeWatchers           atomic.Int32
	watcherChannelBuffer     int
	maxWatchersPerService    int
	adaptiveBufferThreshold  int32
	adaptiveBufferMultiplier int
	maxAdaptiveBuffer        int
	logger                   *slog.Logger
}

func newSubscriptions(o *options) subscriptions {
	shards := make([]watcherShard, o.numShards)
	for i := range shards {
		shards[i].watchers = make(map[string]map[*watcher]struct{})
	}
	return subscriptions{
		watcherShards:            shards,
		watcherChannelBuffer:     o.watcherChannelBuffer,
		maxWatchersPerService:    o.maxWatchersPerService,
		adaptiveBufferThreshold:  o.adaptiveBufferThreshold,
		adaptiveBufferMultiplier: o.adaptiveBufferMultiplier,
		maxAdaptiveBuffer:        o.maxAdaptiveBuffer,
		logger:                   o.logger,
	}
}

func (s *subscriptions) getShardIndex(service string) int {
	// Inline FNV-1a 32-bit to avoid allocating a hash.Hash32 per call.
	const (
		offset32 uint32 = 2166136261
		prime32  uint32 = 16777619
	)
	h := offset32
	for i := range len(service) {
		h ^= uint32(service[i])
		h *= prime32
	}
	return int(h % uint32(len(s.watcherShards))) // #nosec G115 -- numShards is small positive int
}

func (s *subscriptions) subscribe(
	ctx context.Context, service string, initialStatus ServingStatus, closed *atomic.Bool, override *atomic.Int32,
) (Subscriber, error) {
	bufferSize := s.watcherChannelBuffer
	if active := s.activeWatchers.Load(); active > s.adaptiveBufferThreshold {
		bufferSize = s.maxAdaptiveBuffer
		if s.watcherChannelBuffer <= s.maxAdaptiveBuffer/s.adaptiveBufferMultiplier {
			bufferSize = s.watcherChannelBuffer * s.adaptiveBufferMultiplier
		}
	}

	w := newWatcher(bufferSize, initialStatus)
	shardIdx := s.getShardIndex(service)
	shard := &s.watcherShards[shardIdx]

	shard.mu.Lock()
	if closed.Load() {
		shard.mu.Unlock()
		return nil, ErrCoordinatorShutdown
	}
	if len(shard.watchers[service]) >= s.maxWatchersPerService {
		shard.mu.Unlock()
		return nil, ErrWatcherLimitExceeded
	}
	if shard.watchers[service] == nil {
		shard.watchers[service] = make(map[*watcher]struct{})
	}
	var stopCancellation func() bool
	w.remove = func(terminal bool) {
		shard.mu.Lock()
		defer shard.mu.Unlock()
		if stopCancellation != nil {
			stopCancellation()
		}
		w.close(terminal)
		delete(shard.watchers[service], w)
		if len(shard.watchers[service]) == 0 {
			delete(shard.watchers, service)
		}
		s.activeWatchers.Add(-1)
	}
	shard.watchers[service][w] = struct{}{}
	s.activeWatchers.Add(1)

	if closed.Load() {
		shard.mu.Unlock()
		w.stop(true)
		return nil, ErrCoordinatorShutdown
	}
	if status := ServingStatus(override.Load()); status != StatusUnknown && initialStatus != StatusServiceUnknown {
		initialStatus = status
		w.lastStatus = status
	}
	stopCancellation = context.AfterFunc(ctx, func() { w.stop(false) })
	shard.mu.Unlock()

	s.logger.Debug("new subscription created",
		slog.String("service", service),
		slog.Int("buffer_size", bufferSize),
		slog.String("initial_status", initialStatus.String()))

	return &Subscription{initialStatus: initialStatus, updates: w.ch, watcher: w}, nil
}

func (s *subscriptions) notifyWatchers(service string, status ServingStatus) int {
	shardIdx := s.getShardIndex(service)
	shard := &s.watcherShards[shardIdx]

	shard.mu.RLock()
	watchers := shard.watchers[service]
	if watchers == nil {
		shard.mu.RUnlock()
		return 0
	}

	list := make([]*watcher, 0, len(watchers))
	for w := range watchers {
		list = append(list, w)
	}
	shard.mu.RUnlock()

	for _, w := range list {
		w.notify(status)
	}

	return len(list)
}

func (s *subscriptions) logStatusChange(service string, status ServingStatus, count int) {
	if count == 0 {
		return
	}
	s.logger.Debug("status change notified",
		slog.String("service", service),
		slog.String("status", status.String()),
		slog.Int("watchers", count))
}

func (s *subscriptions) broadcast(status ServingStatus) {
	for _, w := range s.snapshot() {
		w.notify(status)
	}
}

func (s *subscriptions) close() {
	for _, w := range s.snapshot() {
		w.stop(true)
	}
}

func (s *subscriptions) snapshot() []*watcher {
	var list []*watcher
	for i := range s.watcherShards {
		shard := &s.watcherShards[i]

		shard.mu.RLock()
		for _, serviceWatchers := range shard.watchers {
			for w := range serviceWatchers {
				list = append(list, w)
			}
		}
		shard.mu.RUnlock()
	}
	return list
}

func (s *subscriptions) count() int {
	totalWatchers := 0
	for i := range s.watcherShards {
		shard := &s.watcherShards[i]
		shard.mu.RLock()
		for _, watchers := range shard.watchers {
			totalWatchers += len(watchers)
		}
		shard.mu.RUnlock()
	}
	return totalWatchers
}

type poll struct {
	watcher *watcher
	pushes  uint64
}

func (s *subscriptions) collectPolls(watchedServices map[string][]poll) {
	for i := range s.watcherShards {
		shard := &s.watcherShards[i]
		shard.mu.RLock()
		for service, watchers := range shard.watchers {
			for w := range watchers {
				watchedServices[service] = append(watchedServices[service], poll{w, w.pushes.Load()})
			}
		}
		shard.mu.RUnlock()
	}
}
