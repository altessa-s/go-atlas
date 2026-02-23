// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/collections/maps"
	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// streamSubscriber implements broker.Subscriber for NATS JetStream.
type streamSubscriber struct {
	consumerConfig   *jetstream.ConsumerConfig
	consumerName     string
	js               jetstream.JetStream
	consumeContext   jetstream.ConsumeContext // Manages the lifecycle of the NATS Consume operation.
	opts             []jetstream.PullConsumeOpt
	handlerCtx       context.Context
	handlerCtxCancel context.CancelFunc
	handlersWg       sync.WaitGroup
	provider         *Nats
}

func newStreamSubscriber(natsProvider *Nats, opt []jetstream.PullConsumeOpt) *streamSubscriber {
	return &streamSubscriber{
		js:       natsProvider.jetStream,
		opts:     opt,
		provider: natsProvider,
	}
}

// SubscriberWithConsumer returns a factory that creates JetStream subscribers with custom consumer config.
// Panics if the provider is not *Nats.
//
// Example:
//
//	factory := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{Durable: "my-consumer"})
//	sub := b.Subscriber(factory)
func SubscriberWithConsumer(consumerConfig *jetstream.ConsumerConfig, opt ...jetstream.PullConsumeOpt) broker.SubscriberFactory {
	return func(p any) broker.Subscriber {
		natsProvider, ok := p.(*Nats)
		panics.Must(ok, "SubscriberWithConsumer factory requires a *natsprovider.Nats instance")

		ss := newStreamSubscriber(natsProvider, opt)
		ss.consumerConfig = consumerConfig
		return ss
	}
}

// SubscriberWithEphemeralConsumer returns a factory that creates JetStream subscribers with ephemeral consumers.
// Panics if the provider is not *Nats.
//
// Example:
//
//	factory := natsprovider.SubscriberWithEphemeralConsumer()
//	sub := b.Subscriber(factory)
func SubscriberWithEphemeralConsumer(opt ...jetstream.PullConsumeOpt) broker.SubscriberFactory {
	return func(p any) broker.Subscriber {
		natsProvider, ok := p.(*Nats)
		panics.Must(ok, "SubscriberWithEphemeralConsumer factory requires a *natsprovider.Nats instance")

		return newStreamSubscriber(natsProvider, opt)
	}
}

// SubscriberWithConsumerName returns a factory that uses an existing named consumer.
// Panics if the provider is not *Nats.
//
// Example:
//
//	factory := natsprovider.SubscriberWithConsumerName("existing-consumer")
//	sub := b.Subscriber(factory)
func SubscriberWithConsumerName(consumerName string, opt ...jetstream.PullConsumeOpt) broker.SubscriberFactory {
	return func(p any) broker.Subscriber {
		natsProvider, ok := p.(*Nats)
		// This panic is for programmer error: StreamSubscriber factory is specific to Nats provider.
		panics.Must(ok, "SubscriberWithConsumerName factory requires a *natsprovider.Nats instance")

		ss := newStreamSubscriber(natsProvider, opt)
		ss.consumerName = consumerName
		return ss
	}
}

// Unsubscribe drains buffered messages, cancels the handler context, and waits
// for all in-flight handlers to complete before returning.
func (ss *streamSubscriber) Unsubscribe() {
	if ss.consumeContext == nil {
		return
	}

	// Drain unsubscribes from the stream and cancels subscription.
	// All messages that are already in the buffer will be processed in callback function.
	ss.consumeContext.Drain() // Drain ensures graceful shutdown.

	// Cancel the handler context and wait for all handlers to finish their work.
	if ss.handlerCtxCancel != nil {
		ss.handlerCtxCancel()
	}
	ss.handlersWg.Wait()
}

// Subscribe creates a JetStream consumer and starts consuming messages for the handler's topic.
func (ss *streamSubscriber) Subscribe(ctx context.Context, handler broker.SubscriberHandler) error {
	if ctx == nil {
		return fmt.Errorf("subscribe: ctx cannot be nil")
	}

	if ss.provider != nil {
		if err := ss.provider.checkSubjectAllowed(handler.Topic()); err != nil {
			return err
		}
	}

	// Always derive a fresh cancellable handler context from the Subscribe ctx.
	// If Subscribe is called again, cancel the previous handler context first.
	if ss.handlerCtxCancel != nil {
		ss.handlerCtxCancel()
	}
	ss.handlerCtx, ss.handlerCtxCancel = context.WithCancel(ctx)

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

	var consumer jetstream.Consumer

	// Create or update the consumer on the stream with the provided configuration.
	// The consumerConfig is from the StreamSubscriber factory.
	if ss.consumerConfig != nil { //nolint:gocritic
		consumer, err = stream.CreateOrUpdateConsumer(ctx, *ss.consumerConfig)
		if err != nil {
			return coreerrs.Wrapf(err, "failed to create/update consumer for stream '%s'", streamName)
		}
	} else if ss.consumerName != "" {
		consumer, err = stream.Consumer(ctx, ss.consumerName)
		if err != nil {
			return coreerrs.Wrapf(err, "failed to get consumer '%s' on stream '%s'", ss.consumerName, streamName)
		}
	} else {
		consumer, err = stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			DeliverPolicy:     jetstream.DeliverLastPolicy,
			InactiveThreshold: time.Hour * 24, //nolint:mnd
			Description:       fmt.Sprintf("Ephemeral consumer for service '%s'", appinfo.Name),
		})
		if err != nil {
			return coreerrs.Wrapf(err, "failed to create/update ephemeral consumer for stream '%s'", streamName)
		}
	}

	// Start consuming messages. This is an asynchronous operation that invokes the callback for each message.
	ss.consumeContext, err = consumer.Consume(func(jsMsg jetstream.Msg) {
		// Filter NATS-specific headers and transform remaining headers to msg.Meta.
		filteredHeaders := maps.FilterMap(jsMsg.Headers(), func(k string, val []string) bool {
			return !strings.HasPrefix(k, "Nats-")
		})

		// Convert filtered headers to msg.MessageMeta slice.
		metaData := make([]msg.MetaData, 0, len(filteredHeaders))
		for _, v := range maps.Map(filteredHeaders, func(k string, v []string) (string, msg.MetaData) {
			return k, msg.MetaData{
				Key:   k,
				Value: strings.Join(v, ";"),
			}
		}) {
			metaData = append(metaData, v)
		}

		// Prepare message options, including the Acker and AckTimeout.
		msgOpts := make([]msg.Option, 0, 2)
		msgOpts = append(msgOpts, msg.WithAcker(&ackAdapter{msg: jsMsg}))
		if consumer.CachedInfo().Config.AckWait > 0 {
			msgOpts = append(msgOpts, msg.WithAckTimeout(consumer.CachedInfo().Config.AckWait))
		}

		// Increment the number of handlers waiting to process messages.
		ss.handlersWg.Add(1)
		func() { // Wrap to ensure wg.Done() and panic handling are always applied.
			defer ss.handlersWg.Done()
			defer panics.Handle(ss.handlerCtx)
			// Create a new message and pass it to the handler.
			handler.Handle(ss.handlerCtx, msg.NewMessageWithMeta(jsMsg.Subject(), jsMsg.Data(), metaData, msgOpts...))
		}()
	}, ss.opts...)

	if err != nil {
		return coreerrs.Wrapf(err, "failed to start consuming from consumer on stream '%s'", streamName)
	}

	return nil
}

// Closed returns a channel that closes when the consumer has fully stopped.
func (ss *streamSubscriber) Closed() <-chan struct{} {
	if ss.consumeContext == nil {
		// If consumeContext was never initialized (e.g., Subscribe failed or was not called),
		// return a pre-closed channel to prevent blocking.
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return ss.consumeContext.Closed()
}
