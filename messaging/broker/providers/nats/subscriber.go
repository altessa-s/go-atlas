// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// EphemeralInactiveThreshold is how long the server keeps an ephemeral
// consumer after its subscriber stops pulling. An active Consume keeps the
// consumer alive, so this only bounds how long abandoned consumers (crashed
// or unsubscribed instances) linger on the server.
const EphemeralInactiveThreshold = 5 * time.Minute

// ErrSubscribeSuperseded means a later Subscribe or Unsubscribe superseded a
// Subscribe while it waited for the previous consumer to drain.
var ErrSubscribeSuperseded = errors.New("subscribe superseded by a later lifecycle transition")

// streamSubscriber implements broker.Subscriber for NATS JetStream.
type streamSubscriber struct {
	consumerConfig *jetstream.ConsumerConfig
	consumerName   string
	js             jetstream.JetStream
	opts           []jetstream.PullConsumeOpt
	handlersWg     sync.WaitGroup
	provider       *NATS
	metrics        *subscriberMetrics
	drainTimeout   time.Duration // grace for buffered handlers on Unsubscribe; <= 0 = none

	// mu guards the lifecycle fields below, which Subscribe writes and
	// Unsubscribe/Closed read from potentially different goroutines.
	generation       uint64
	draining         chan struct{}
	mu               sync.Mutex
	consumeContext   jetstream.ConsumeContext // Manages the lifecycle of the NATS Consume operation.
	handlerCtxCancel context.CancelFunc
}

func newStreamSubscriber(natsProvider *NATS, opt []jetstream.PullConsumeOpt) *streamSubscriber {
	return &streamSubscriber{
		js:           natsProvider.jetStream,
		opts:         opt,
		provider:     natsProvider,
		metrics:      newSubscriberMetrics(natsProvider.meter),
		drainTimeout: natsProvider.drainTimeout,
	}
}

// SubscriberFactory creates a JetStream subscriber for the supplied NATS provider.
type SubscriberFactory func(*NATS) broker.Subscriber

// SubscriberWithConsumer returns a factory that creates JetStream subscribers with custom consumer config.
//
// Example:
//
//	factory := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{Durable: "my-consumer"})
//	sub := provider.Subscriber(factory)
func SubscriberWithConsumer(consumerConfig *jetstream.ConsumerConfig, opt ...jetstream.PullConsumeOpt) SubscriberFactory {
	return func(natsProvider *NATS) broker.Subscriber {
		ss := newStreamSubscriber(natsProvider, opt)
		ss.consumerConfig = consumerConfig
		return ss
	}
}

// SubscriberWithEphemeralConsumer returns a factory that creates JetStream subscribers with ephemeral consumers.
//
// Example:
//
//	factory := natsprovider.SubscriberWithEphemeralConsumer()
//	sub := provider.Subscriber(factory)
func SubscriberWithEphemeralConsumer(opt ...jetstream.PullConsumeOpt) SubscriberFactory {
	return func(natsProvider *NATS) broker.Subscriber {
		return newStreamSubscriber(natsProvider, opt)
	}
}

// SubscriberWithConsumerName returns a factory that uses an existing named consumer.
//
// Example:
//
//	factory := natsprovider.SubscriberWithConsumerName("existing-consumer")
//	sub := provider.Subscriber(factory)
func SubscriberWithConsumerName(consumerName string, opt ...jetstream.PullConsumeOpt) SubscriberFactory {
	return func(natsProvider *NATS) broker.Subscriber {
		ss := newStreamSubscriber(natsProvider, opt)
		ss.consumerName = consumerName
		return ss
	}
}

// ConsumerName returns the configured, stable name used for consumer tracing.
func (ss *streamSubscriber) ConsumerName() string {
	if ss.consumerConfig != nil {
		return cmp.Or(ss.consumerConfig.Durable, ss.consumerConfig.Name)
	}
	return ss.consumerName
}

// Unsubscribe drains buffered messages, waits for their handlers for up to
// the provider's drain timeout ([WithDrainTimeout]), then cancels the handler
// context and waits until the consumer has fully stopped and all handlers,
// including those for messages still buffered when the drain started, have
// completed.
func (ss *streamSubscriber) Unsubscribe() {
	ss.beginTransition()
	ss.mu.Unlock()
}

// beginTransition returns with mu held after the previous consumer drains.
// A newer transition supersedes a caller while it waits for that drain.
func (ss *streamSubscriber) beginTransition() bool {
	ss.mu.Lock()
	ss.generation++
	generation := ss.generation
	for ss.draining != nil {
		done := ss.draining
		ss.mu.Unlock()
		<-done
		ss.mu.Lock()
		if generation != ss.generation {
			return false
		}
	}
	previous, cancel := ss.consumeContext, ss.handlerCtxCancel
	ss.consumeContext, ss.handlerCtxCancel = nil, nil
	if previous != nil {
		done := make(chan struct{})
		ss.draining = done
		ss.mu.Unlock()
		ss.stopConsume(previous, cancel)
		ss.mu.Lock()
		ss.draining = nil
		close(done)
	} else if cancel != nil {
		cancel()
	}
	return generation == ss.generation
}

// stopConsume drains consumeContext and returns once it has closed and every
// handler has returned.
func (ss *streamSubscriber) stopConsume(consumeContext jetstream.ConsumeContext, cancel context.CancelFunc) {
	// Drain unsubscribes from the stream and cancels subscription.
	// All messages that are already in the buffer will be processed in callback function.
	consumeContext.Drain() // Drain ensures graceful shutdown.

	// Canceling the handler context right away would run the buffered
	// messages with a canceled context, so they would be Nak'd instead of
	// processed. Drain is asynchronous: the consume context closes after the
	// last callback returned, which is what the grace period waits for.
	if ss.drainTimeout > 0 {
		grace := time.NewTimer(ss.drainTimeout)
		select {
		case <-consumeContext.Closed():
		case <-grace.C:
		}
		grace.Stop()
	}

	// Cancel the handler context and wait for all handlers to finish their work.
	if cancel != nil {
		cancel()
	}
	// Buffered callbacks join handlersWg only when they start, so first wait
	// for the consume context to close (after the last callback returned); the
	// WaitGroup still covers a callback in flight when the connection closes
	// and Closed() reports closed immediately.
	<-consumeContext.Closed()
	ss.handlersWg.Wait()
}

// Subscribe creates a JetStream consumer and starts consuming messages for the handler's topic.
// Calling it again replaces the subscription: the previous one is stopped as
// by Unsubscribe before the new consumer starts. It returns [ErrSubscribeSuperseded]
// if a later Subscribe or Unsubscribe supersedes it while draining.
func (ss *streamSubscriber) Subscribe(ctx context.Context, handler broker.SubscriberHandler) error {
	if ctx == nil {
		return fmt.Errorf("subscribe: ctx cannot be nil")
	}

	if ss.provider != nil {
		if err := ss.provider.checkSubscriptionAllowed(handler.Topic()); err != nil {
			return err
		}
	}

	current := ss.beginTransition()
	defer ss.mu.Unlock()
	if !current {
		return ErrSubscribeSuperseded
	}
	handlerCtx, handlerCtxCancel := context.WithCancel(ctx)
	defer func() {
		if ss.consumeContext == nil {
			handlerCtxCancel()
		}
	}()

	// Determine the JetStream stream name based on the subject (topic).
	streamName, err := ss.js.StreamNameBySubject(ctx, handler.Topic())
	if err != nil {
		return coreerrs.Wrapf(err, "failed to get stream name for subject '%s'", handler.Topic())
	}

	// Get the stream.
	stream, err := ss.js.Stream(ctx, streamName)
	if err != nil {
		return coreerrs.Wrapf(err, "failed to get stream '%s'", streamName)
	}

	consumer, err := ss.resolveConsumer(ctx, stream, streamName, handler.Topic())
	if err != nil {
		return err
	}

	// Start consuming messages. This is an asynchronous operation that invokes the callback for each message.
	// Cache consumer config outside the hot loop — AckWait is static per consumer.
	ackWait := consumer.CachedInfo().Config.AckWait

	// Label by the subscription subject, not the per-message subject: a
	// wildcard subscription (orders.*.created) would otherwise mint one series
	// per concrete subject (e.g. per tenant). Handles are bound once here.
	subjectLabels := metrics.Labels{labelSubject: handler.Topic()}
	received := ss.metrics.messagesReceived.WithLabels(subjectLabels)
	duration := ss.metrics.processingDuration.WithLabels(subjectLabels)
	failures := ss.metrics.processingErrors.WithLabels(subjectLabels)
	onFail := failures.Inc

	consumeContext, err := consumer.Consume(func(jsMsg jetstream.Msg) {
		metaData := messageMetadata(jsMsg.Headers())

		// Prepare message options, including the Acker and AckTimeout.
		msgOpts := make([]msg.Option, 0, 2)
		acker := &ackAdapter{msg: jsMsg, onFail: onFail}
		msgOpts = append(msgOpts, msg.WithAcker(acker))
		msgOpts = slices.AppendIf(msgOpts, ackWait > 0, msg.WithAckTimeout(ackWait))

		// Inline defers instead of IIFE to avoid closure allocation per message.
		ss.handlersWg.Add(1)
		defer ss.handlersWg.Done()
		// A panicking handler counts as a processing failure; markFailed
		// dedupes it against a Nak/Term issued before the panic.
		defer panics.Handle(handlerCtx, func(context.Context, any) { acker.markFailed() })

		received.Inc()
		stopTimer := duration.Start()
		handler.Handle(handlerCtx, msg.NewMessageWithMeta(jsMsg.Subject(), jsMsg.Data(), metaData, msgOpts...))
		stopTimer()
	}, ss.opts...)
	if err != nil {
		return coreerrs.Wrapf(err, "failed to start consuming from consumer on stream '%s'", streamName)
	}

	ss.consumeContext, ss.handlerCtxCancel = consumeContext, handlerCtxCancel

	return nil
}

func messageMetadata(headers nats.Header) msg.Meta {
	metaData := make([]msg.MetaData, 0, len(headers))
	for k, vals := range headers {
		if strings.HasPrefix(k, "Nats-") {
			continue
		}
		separator := ";"
		if k == "tracestate" {
			separator = ","
		}
		metaData = append(metaData, msg.MetaData{Key: k, Value: strings.Join(vals, separator)})
	}
	return appendDeduplicateID(metaData, headers)
}

// appendDeduplicateID restores the [msg.MetaKeyDeduplicateId] metadata the
// publisher moved into the JetStream Nats-Msg-Id header (see buildNatsMsg),
// so consumers such as the broker idempotency middleware can key on it. An
// application header carrying the key itself, even an empty one, takes
// precedence and the Nats-Msg-Id is then not added.
func appendDeduplicateID(metaData []msg.MetaData, headers nats.Header) []msg.MetaData {
	ids := headers[jetstream.MsgIDHeader]
	if len(ids) == 0 || ids[0] == "" {
		return metaData
	}
	if _, explicit := headers[msg.MetaKeyDeduplicateId]; explicit {
		return metaData
	}
	return append(metaData, msg.MetaData{Key: msg.MetaKeyDeduplicateId, Value: ids[0]})
}

// Closed returns a channel that closes when the consumer has fully stopped.
func (ss *streamSubscriber) Closed() <-chan struct{} {
	ss.mu.Lock()
	consumeContext := ss.consumeContext
	draining := ss.draining
	ss.mu.Unlock()

	if consumeContext == nil {
		if draining != nil {
			return draining
		}
		// If consumeContext was never initialized (e.g., Subscribe failed or was not called),
		// return a pre-closed channel to prevent blocking.
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return consumeContext.Closed()
}
