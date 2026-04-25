// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/nats-io/nats.go/jetstream"
)

// HealthMonitor performs health checks on registered streams and consumers.
// It serves as a fallback mechanism to catch deletions that may have been
// missed by the [AdvisoryListener] (e.g., during NATS reconnection or
// network issues).
//
// The monitor is designed to be used with an external scheduler.
// Call [HealthMonitor.Run] to perform a single health check cycle.
type HealthMonitor struct {
	js         jetstream.JetStream
	registry   *Registry
	supervisor *Supervisor
	logger     *slog.Logger
}

// HealthMonitorConfig holds configuration for the HealthMonitor.
type HealthMonitorConfig struct {
	JetStream  jetstream.JetStream
	Registry   *Registry
	Supervisor *Supervisor
	Logger     *slog.Logger
}

// NewHealthMonitor creates a new HealthMonitor.
func NewHealthMonitor(cfg HealthMonitorConfig) *HealthMonitor {
	return &HealthMonitor{
		js:         cfg.JetStream,
		registry:   cfg.Registry,
		supervisor: cfg.Supervisor,
		logger:     cfg.Logger,
	}
}

// Run performs a single health check cycle on all registered streams and consumers.
// Compatible with scheduler.TaskFunc signature for use with external scheduler.
func (h *HealthMonitor) Run(ctx context.Context) error {
	h.check(ctx)
	return nil
}

// check performs a single health check cycle on all registered streams and consumers.
func (h *HealthMonitor) check(ctx context.Context) {
	// Collect stream names to avoid holding lock during checks.
	streamNames := slices.Collect(h.registry.StreamNames())

	if len(streamNames) == 0 {
		return
	}

	h.logger.Debug("running health check",
		slog.Int("streams", len(streamNames)))

	for _, streamName := range streamNames {
		select {
		case <-ctx.Done():
			return
		default:
		}

		h.checkStream(ctx, streamName)
	}
}

// checkStream checks if a stream exists and triggers recovery if not found.
func (h *HealthMonitor) checkStream(ctx context.Context, streamName string) {
	// Check if stream exists.
	_, err := h.js.Stream(ctx, streamName)
	if err != nil {
		if isNotFoundError(err) {
			h.logger.Warn("stream not found during health check",
				slog.String("stream", streamName))
			h.supervisor.RecoverStream(streamName, nil)
			return
		}

		// Log other errors but don't trigger recovery.
		h.logger.Warn("failed to check stream health",
			slog.String("stream", streamName),
			slog.Any("error", err))
		return
	}

	// Stream exists, check consumers.
	h.checkConsumers(ctx, streamName)
}

// checkConsumers checks if all registered consumers for a stream exist.
func (h *HealthMonitor) checkConsumers(ctx context.Context, streamName string) {
	consumerNames := slices.Collect(h.registry.ConsumerNames(streamName))

	for _, consumerName := range consumerNames {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, err := h.js.Consumer(ctx, streamName, consumerName)
		if err != nil {
			if isNotFoundError(err) {
				h.logger.Warn("consumer not found during health check",
					slog.String("stream", streamName),
					slog.String("consumer", consumerName))
				h.supervisor.RecoverConsumer(streamName, consumerName, nil)
				continue
			}

			// Log other errors but don't trigger recovery.
			h.logger.Warn("failed to check consumer health",
				slog.String("stream", streamName),
				slog.String("consumer", consumerName),
				slog.Any("error", err))
		}
	}
}

// isNotFoundError checks if the error indicates the resource was not found.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, jetstream.ErrStreamNotFound) ||
		errors.Is(err, jetstream.ErrConsumerNotFound)
}
