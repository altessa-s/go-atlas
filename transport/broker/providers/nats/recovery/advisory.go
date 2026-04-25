// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"

	"github.com/nats-io/nats.go"
)

// JetStream advisory event topics and subject parsing constants.
const (
	// Advisory topic for stream deletion events.
	advisoryStreamDeleted = "$JS.EVENT.ADVISORY.STREAM.DELETED.>"

	// Advisory topic for consumer deletion events.
	advisoryConsumerDeleted = "$JS.EVENT.ADVISORY.CONSUMER.DELETED.>"

	// minStreamSubjectParts is the minimum number of parts in a stream advisory subject.
	// Format: $JS.EVENT.ADVISORY.STREAM.DELETED.<stream>
	minStreamSubjectParts = 6

	// minConsumerSubjectParts is the minimum number of parts in a consumer advisory subject.
	// Format: $JS.EVENT.ADVISORY.CONSUMER.DELETED.<stream>.<consumer>
	minConsumerSubjectParts = 7
)

// StreamDeletedAdvisory represents the JetStream advisory event for stream deletion.
type StreamDeletedAdvisory struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Time   string `json:"time"`
	Stream string `json:"stream"`
	Domain string `json:"domain,omitempty"`
}

// ConsumerDeletedAdvisory represents the JetStream advisory event for consumer deletion.
type ConsumerDeletedAdvisory struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Time     string `json:"time"`
	Stream   string `json:"stream"`
	Consumer string `json:"consumer"`
	Domain   string `json:"domain,omitempty"`
}

// AdvisoryListener subscribes to JetStream advisory events for stream and consumer deletion.
// When a deletion event is received for a registered stream or consumer,
// it triggers recovery through the [Supervisor].
//
// All exported methods are safe for concurrent use.
type AdvisoryListener struct {
	nc         *nats.Conn
	registry   *Registry
	supervisor *Supervisor
	logger     *slog.Logger

	mu   sync.Mutex
	subs []*nats.Subscription
}

// AdvisoryListenerConfig holds configuration for the AdvisoryListener.
type AdvisoryListenerConfig struct {
	NatsConn   *nats.Conn
	Registry   *Registry
	Supervisor *Supervisor
	Logger     *slog.Logger
}

// NewAdvisoryListener creates a new AdvisoryListener.
func NewAdvisoryListener(cfg AdvisoryListenerConfig) *AdvisoryListener {
	return &AdvisoryListener{
		nc:         cfg.NatsConn,
		registry:   cfg.Registry,
		supervisor: cfg.Supervisor,
		logger:     cfg.Logger,
	}
}

// Start subscribes to JetStream advisory events.
// Call Stop to unsubscribe when done.
func (a *AdvisoryListener) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Subscribe to stream deletion events.
	streamSub, err := a.nc.Subscribe(advisoryStreamDeleted, a.handleStreamDeleted)
	if err != nil {
		return err
	}

	// Subscribe to consumer deletion events.
	consumerSub, err := a.nc.Subscribe(advisoryConsumerDeleted, a.handleConsumerDeleted)
	if err != nil {
		_ = streamSub.Unsubscribe() //nolint:errcheck // Best-effort cleanup
		return err
	}

	a.subs = []*nats.Subscription{streamSub, consumerSub}

	a.logger.Info("advisory listener started",
		slog.String("streamTopic", advisoryStreamDeleted),
		slog.String("consumerTopic", advisoryConsumerDeleted))

	return nil
}

// Stop unsubscribes from all advisory events and drains pending messages.
func (a *AdvisoryListener) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var lastErr error
	for _, sub := range a.subs {
		if err := sub.Drain(); err != nil {
			lastErr = err
			a.logger.Warn("failed to drain subscription",
				slog.String("subject", sub.Subject),
				slog.Any("error", err))
		}
	}

	a.subs = nil
	a.logger.Info("advisory listener stopped")

	return lastErr
}

// handleStreamDeleted processes stream deletion advisory events.
func (a *AdvisoryListener) handleStreamDeleted(msg *nats.Msg) {
	var event StreamDeletedAdvisory
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		a.logger.Warn("failed to parse stream deleted advisory",
			slog.Any("error", err),
			slog.String("subject", msg.Subject))
		return
	}

	// Extract stream name from subject if not in payload.
	streamName := event.Stream
	if streamName == "" {
		streamName = extractStreamNameFromSubject(msg.Subject)
	}

	if streamName == "" {
		a.logger.Warn("stream name not found in advisory",
			slog.String("subject", msg.Subject))
		return
	}

	a.logger.Info("stream deleted advisory received",
		slog.String("stream", streamName),
		slog.String("id", event.ID))

	// Check if stream is registered and trigger recovery.
	if a.registry.HasStream(streamName) {
		a.supervisor.RecoverStream(streamName, &event)
	} else {
		a.logger.Debug("ignoring deletion for unregistered stream",
			slog.String("stream", streamName))
	}
}

// handleConsumerDeleted processes consumer deletion advisory events.
func (a *AdvisoryListener) handleConsumerDeleted(msg *nats.Msg) {
	var event ConsumerDeletedAdvisory
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		a.logger.Warn("failed to parse consumer deleted advisory",
			slog.Any("error", err),
			slog.String("subject", msg.Subject))
		return
	}

	// Extract stream and consumer names from subject if not in payload.
	streamName := event.Stream
	consumerName := event.Consumer
	if streamName == "" || consumerName == "" {
		s, c := extractConsumerNamesFromSubject(msg.Subject)
		if streamName == "" {
			streamName = s
		}
		if consumerName == "" {
			consumerName = c
		}
	}

	if streamName == "" || consumerName == "" {
		a.logger.Warn("stream/consumer name not found in advisory",
			slog.String("subject", msg.Subject))
		return
	}

	a.logger.Info("consumer deleted advisory received",
		slog.String("stream", streamName),
		slog.String("consumer", consumerName),
		slog.String("id", event.ID))

	// Check if consumer is registered and trigger recovery.
	if a.registry.HasConsumer(streamName, consumerName) {
		a.supervisor.RecoverConsumer(streamName, consumerName, &event)
	} else {
		a.logger.Debug("ignoring deletion for unregistered consumer",
			slog.String("stream", streamName),
			slog.String("consumer", consumerName))
	}
}

// extractStreamNameFromSubject extracts stream name from advisory subject.
// Subject format: $JS.EVENT.ADVISORY.STREAM.DELETED.<stream>
func extractStreamNameFromSubject(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) >= minStreamSubjectParts {
		return parts[5]
	}
	return ""
}

// extractConsumerNamesFromSubject extracts stream and consumer names from advisory subject.
// Subject format: $JS.EVENT.ADVISORY.CONSUMER.DELETED.<stream>.<consumer>
func extractConsumerNamesFromSubject(subject string) (stream, consumer string) {
	parts := strings.Split(subject, ".")
	if len(parts) >= minConsumerSubjectParts {
		return parts[5], parts[6]
	}
	return "", ""
}
