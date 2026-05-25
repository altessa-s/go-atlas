// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
)

// recoveryState tracks the state of an ongoing recovery operation.
type recoveryState struct {
	startedAt time.Time
	attempts  int
}

// Supervisor handles stream and consumer recovery with linear backoff (backoff * attempt).
// It coordinates recovery operations and prevents concurrent recovery of the same stream
// using an internal recovering-state map protected by a mutex.
//
// All exported methods are safe for concurrent use.
type Supervisor struct {
	js       jetstream.JetStream
	registry *Registry
	logger   *slog.Logger

	maxAttempts int
	backoff     time.Duration

	onRecoverySuccess func(stream, consumer string)
	onRecoveryFailure func(stream, consumer string, err error)
	onManualNeeded    func(stream string, event any)
	onStaleCleared    func(stream string)

	// recoveringStreams tracks streams currently being recovered.
	recoveringMu     sync.Mutex
	recoveringStates map[string]*recoveryState

	ctx    context.Context
	cancel context.CancelFunc
}

// SupervisorConfig holds configuration for the Supervisor.
type SupervisorConfig struct {
	JetStream         jetstream.JetStream
	Registry          *Registry
	Logger            *slog.Logger
	MaxAttempts       int
	Backoff           time.Duration
	OnRecoverySuccess func(stream, consumer string)
	OnRecoveryFailure func(stream, consumer string, err error)
	OnManualNeeded    func(stream string, event any)
	OnStaleCleared    func(stream string)
}

// NewSupervisor creates a new Supervisor with the given configuration.
// The supervisor owns a detached background context (cancelled by
// [Supervisor.Stop]) because its watcher goroutines must outlive any
// request-scoped context the caller might have. Termination is honest:
// Stop() invokes the stored cancel — no goroutine leaks once the
// supervisor is asked to stop.
func NewSupervisor(cfg SupervisorConfig) *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())

	return &Supervisor{
		js:                cfg.JetStream,
		registry:          cfg.Registry,
		logger:            cfg.Logger,
		maxAttempts:       cfg.MaxAttempts,
		backoff:           cfg.Backoff,
		onRecoverySuccess: cfg.OnRecoverySuccess,
		onRecoveryFailure: cfg.OnRecoveryFailure,
		onManualNeeded:    cfg.OnManualNeeded,
		onStaleCleared:    cfg.OnStaleCleared,
		recoveringStates:  make(map[string]*recoveryState),
		ctx:               ctx,
		cancel:            cancel,
	}
}

// Close stops the supervisor and cancels any ongoing recovery operations.
func (s *Supervisor) Close() {
	s.cancel()
}

// markAsRecovering marks a stream as being recovered.
// Returns false if already recovering.
func (s *Supervisor) markAsRecovering(stream string) bool {
	s.recoveringMu.Lock()
	defer s.recoveringMu.Unlock()

	if _, ok := s.recoveringStates[stream]; ok {
		return false
	}

	s.recoveringStates[stream] = &recoveryState{
		startedAt: time.Now(),
		attempts:  0,
	}
	return true
}

// clearRecoveringMark removes the recovering mark for a stream.
func (s *Supervisor) clearRecoveringMark(stream string) {
	s.recoveringMu.Lock()
	defer s.recoveringMu.Unlock()

	delete(s.recoveringStates, stream)
}

// GetRecoveringStreams returns a list of streams currently being recovered.
func (s *Supervisor) GetRecoveringStreams() []string {
	s.recoveringMu.Lock()
	defer s.recoveringMu.Unlock()

	streams := make([]string, 0, len(s.recoveringStates))
	for stream := range s.recoveringStates {
		streams = append(streams, stream)
	}
	return streams
}

// ClearStaleRecoveries removes recovery marks older than timeout, unblocking future
// recovery attempts for those streams. Returns the names of cleared streams.
// Invokes OnStaleCleared callback for each cleared stream.
func (s *Supervisor) ClearStaleRecoveries(timeout time.Duration) []string {
	s.recoveringMu.Lock()
	defer s.recoveringMu.Unlock()

	var cleared []string
	now := time.Now()

	for stream, state := range s.recoveringStates {
		if now.Sub(state.startedAt) > timeout {
			delete(s.recoveringStates, stream)
			cleared = append(cleared, stream)

			s.logger.Warn("cleared stale recovery mark",
				slog.String("stream", stream),
				slog.Duration("age", now.Sub(state.startedAt)))

			if s.onStaleCleared != nil {
				s.onStaleCleared(stream)
			}
		}
	}

	return cleared
}

// RecoverStream initiates recovery of a deleted stream according to its [RecoveryStrategy].
// For [RecoveryStrategyAuto], recovery runs asynchronously in a new goroutine with retries.
// The event parameter carries the advisory payload (may be nil for health-check triggers).
// If recovery is already in progress for this stream, the call is a no-op.
func (s *Supervisor) RecoverStream(streamName string, event any) {
	// Check recovery strategy first.
	strategy := s.registry.GetRecoveryStrategy(streamName)

	switch strategy {
	case RecoveryStrategySkip:
		s.logger.Debug("skipping recovery for stream",
			slog.String("stream", streamName),
			slog.String("strategy", strategy.String()))
		return

	case RecoveryStrategyManual:
		s.logger.Info("manual recovery needed for stream",
			slog.String("stream", streamName))
		if s.onManualNeeded != nil {
			s.onManualNeeded(streamName, event)
		}
		return

	case RecoveryStrategyAuto:
		// Continue with automatic recovery below.
	}

	// Prevent concurrent recovery of same stream.
	if !s.markAsRecovering(streamName) {
		s.logger.Debug("recovery already in progress",
			slog.String("stream", streamName))
		return
	}

	// Run recovery in a goroutine to not block the caller.
	go s.runStreamRecovery(streamName)
}

// runStreamRecovery performs the actual stream recovery with retries.
func (s *Supervisor) runStreamRecovery(streamName string) {
	defer s.clearRecoveringMark(streamName)

	cfg, ok := s.registry.GetStreamConfig(streamName)
	if !ok {
		s.logger.Error("stream config not found in registry",
			slog.String("stream", streamName))
		return
	}

	err := coreretry.Do(s.ctx, func(ctx context.Context) error {
		// Recreate stream.
		if _, err := s.js.CreateOrUpdateStream(ctx, cfg); err != nil && !isAlreadyExistsError(err) {
			return coreerrs.WrapOperation(err, "recreate stream")
		}
		// Restore subscriptions.
		if err := s.restoreSubscriptions(streamName); err != nil {
			return coreerrs.WrapOperation(err, "restore subscriptions")
		}
		return nil
	}, s.retryOpts(streamName, "")...)

	if err != nil {
		s.logger.Error("stream recovery failed after max attempts",
			slog.String("stream", streamName),
			slog.Int("maxAttempts", s.maxAttempts),
			slog.Any("lastError", err))
		if s.onRecoveryFailure != nil {
			s.onRecoveryFailure(streamName, "", err)
		}
		return
	}

	s.logger.Info("stream recovery successful",
		slog.String("stream", streamName))
	if s.onRecoverySuccess != nil {
		s.onRecoverySuccess(streamName, "")
	}
}

// RecoverConsumer initiates recovery of a deleted consumer according to its stream's [RecoveryStrategy].
// For [RecoveryStrategyAuto], recovery runs asynchronously in a new goroutine with retries.
// The event parameter carries the advisory payload (may be nil for health-check triggers).
func (s *Supervisor) RecoverConsumer(stream, consumer string, event any) {
	// Check stream recovery strategy.
	strategy := s.registry.GetRecoveryStrategy(stream)

	switch strategy {
	case RecoveryStrategySkip:
		s.logger.Debug("skipping consumer recovery",
			slog.String("stream", stream),
			slog.String("consumer", consumer),
			slog.String("strategy", strategy.String()))
		return

	case RecoveryStrategyManual:
		s.logger.Info("manual recovery needed for consumer",
			slog.String("stream", stream),
			slog.String("consumer", consumer))
		if s.onManualNeeded != nil {
			s.onManualNeeded(stream, event)
		}
		return

	case RecoveryStrategyAuto:
		// Continue with automatic recovery below.
	}

	// Run recovery in a goroutine.
	go s.runConsumerRecovery(stream, consumer)
}

// runConsumerRecovery performs the actual consumer recovery with retries.
// The resubscribe handler (ManagedSubscription.resubscribe) will recreate
// the consumer via the factory.
func (s *Supervisor) runConsumerRecovery(stream, consumer string) {
	handler, ok := s.registry.GetResubscribeHandler(stream, consumer)
	if !ok {
		s.logger.Error("resubscribe handler not found in registry",
			slog.String("stream", stream),
			slog.String("consumer", consumer))
		return
	}

	err := coreretry.Do(s.ctx, func(_ context.Context) error {
		if err := handler(); err != nil {
			return coreerrs.WrapOperation(err, "resubscribe")
		}
		return nil
	}, s.retryOpts(stream, consumer)...)

	if err != nil {
		s.logger.Error("consumer recovery failed after max attempts",
			slog.String("stream", stream),
			slog.String("consumer", consumer),
			slog.Int("maxAttempts", s.maxAttempts),
			slog.Any("lastError", err))
		if s.onRecoveryFailure != nil {
			s.onRecoveryFailure(stream, consumer, err)
		}
		return
	}

	s.logger.Info("consumer recovery successful",
		slog.String("stream", stream),
		slog.String("consumer", consumer))
	if s.onRecoverySuccess != nil {
		s.onRecoverySuccess(stream, consumer)
	}
}

// restoreSubscriptions resubscribes all subscriptions for a stream.
func (s *Supervisor) restoreSubscriptions(stream string) error {
	subscriptions := s.registry.GetStreamSubscriptions(stream)

	for _, entry := range subscriptions {
		// Get resubscribe handler and call it.
		handler, ok := s.registry.GetResubscribeHandler(stream, entry.consumer)
		if !ok {
			s.logger.Warn("resubscribe handler not found",
				slog.String("stream", stream),
				slog.String("consumer", entry.consumer))
			continue
		}

		if err := handler(); err != nil {
			return coreerrs.Wrapf(err, "resubscribe %s", entry.consumer)
		}

		s.logger.Debug("subscription restored",
			slog.String("stream", stream),
			slog.String("consumer", entry.consumer))

		if s.onRecoverySuccess != nil {
			s.onRecoverySuccess(stream, entry.consumer)
		}
	}

	return nil
}

// retryOpts returns the shared retry options for stream and consumer
// recovery loops: linear backoff (backoff * (attempt+1)), capped at
// maxAttempts, with per-attempt warning logs.
func (s *Supervisor) retryOpts(stream, consumer string) []coreretry.Option {
	return []coreretry.Option{
		coreretry.WithMaxAttempts(s.maxAttempts - 1), // retry.Do counts from 0; attempt 0 is the first try
		coreretry.WithNextDelay(func(attempt int, _ error) time.Duration {
			return s.backoff * time.Duration(attempt+1)
		}),
		coreretry.WithOnRetry(func(attempt int, err error, _ time.Duration) {
			s.logger.Warn("recovery attempt failed, retrying",
				slog.String("stream", stream),
				slog.String("consumer", consumer),
				slog.Int("attempt", attempt+1),
				slog.Int("maxAttempts", s.maxAttempts),
				slog.Any("error", err))
		}),
	}
}

// isAlreadyExistsError checks if the error indicates the resource already exists.
func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}

	// JetStream returns specific errors for already existing resources.
	return errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) ||
		errors.Is(err, jetstream.ErrConsumerNameAlreadyInUse)
}
