// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	"google.golang.org/protobuf/proto"
)

// StorageKeyPrefix is the domain tag of the storage keys managed by this package: the first component of the tuple
// [BuildStorageKey] composes, which keeps them apart from the keys of the HTTP and gRPC transports.
const StorageKeyPrefix = "ibk"

// KeyFunc extracts an idempotency key from an incoming message.
// Returning an empty string without error passes the message through to the inner handler without deduplication.
// A non-nil error terminates the message with [msg.Message.Term].
type KeyFunc func(*msg.Message) (string, error)

// DefaultKeyFunc extracts the idempotency key from message metadata:
// first [msg.MetaKeyDeduplicateId], then [msg.MetaKeyMessageId].
// Returns an empty string and nil error if neither key is set or non-empty.
func DefaultKeyFunc(m *msg.Message) (string, error) {
	if m == nil {
		return "", nil
	}
	if v, ok := m.Metadata.Value(msg.MetaKeyDeduplicateId); ok && v != "" {
		return v, nil
	}
	if v, ok := m.Metadata.Value(msg.MetaKeyMessageId); ok && v != "" {
		return v, nil
	}
	return "", nil
}

// ProtoKeyFunc builds a KeyFunc that unmarshals m.Data into a fresh protobuf message
// of type M and extracts the idempotency key using get.
// An unmarshal failure returns a non-nil error, causing the message to be terminated.
func ProtoKeyFunc[M proto.Message](newMsg func() M, get func(M) string) KeyFunc {
	if newMsg == nil || get == nil {
		panic("idempotency: newMsg and get cannot be nil")
	}
	return func(m *msg.Message) (string, error) {
		if m == nil {
			return "", nil
		}
		pb := newMsg()
		if err := proto.Unmarshal(m.Data, pb); err != nil {
			return "", err
		}
		return get(pb), nil
	}
}

// BuildStorageKey formats the storage key for an idempotency record: the tuple ([StorageKeyPrefix], consumer, key)
// composed by [idempotency.ComposeKey], so distinct (consumer, key) pairs never share a storage key whatever bytes they
// hold. The result persists in storage backends; see [idempotency.ComposeKey] for the frozen v2 layout.
func BuildStorageKey(consumer, key string) string {
	return idempotency.ComposeKey(StorageKeyPrefix, consumer, key)
}

// Wrap returns a [broker.SubscriberHandler] that deduplicates incoming messages
// for the given handler h using keeper.
//
// The consumer string scopes keys so two consumers of the same topic do not collide.
// If h is nil, keeper is nil, or consumer is empty, Wrap panics.
func Wrap(h broker.SubscriberHandler, keeper *idempotency.Keeper, consumer string, opt ...Option) broker.SubscriberHandler {
	if h == nil {
		panic("idempotency: handler cannot be nil")
	}
	if keeper == nil {
		panic("idempotency: keeper cannot be nil")
	}
	if consumer == "" {
		panic("idempotency: consumer cannot be empty")
	}

	options := newOptions(opt...)
	logger := options.logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &wrappedHandler{
		handler:  h,
		keeper:   keeper,
		consumer: consumer,
		opts:     options,
		metrics:  newBrokerMetrics(options.collector, consumer),
		logger:   logger,
	}
}

type wrappedHandler struct {
	handler  broker.SubscriberHandler
	keeper   *idempotency.Keeper
	consumer string
	opts     *options
	metrics  *brokerMetrics
	logger   *slog.Logger
}

// Topic delegates to the underlying handler's Topic method.
func (w *wrappedHandler) Topic() string {
	return w.handler.Topic()
}

// Handle executes idempotency check before invoking the inner handler.
func (w *wrappedHandler) Handle(ctx context.Context, m *msg.Message) {
	if m == nil {
		return
	}

	key, err := w.opts.keyFunc(m)
	if err != nil {
		w.logger.ErrorContext(ctx, "idempotency: key extraction failed",
			slog.String("consumer", w.consumer),
			slog.String("topic", w.handler.Topic()),
			slog.String("error", err.Error()),
		)
		w.metrics.inc(outcomeKeyError)
		_ = m.Term("idempotency: " + err.Error())
		return
	}

	if key == "" {
		w.logger.WarnContext(ctx, "idempotency: empty key; passing message through without deduplication",
			slog.String("consumer", w.consumer),
			slog.String("topic", w.handler.Topic()),
		)
		w.metrics.inc(outcomeNoKey)
		w.handler.Handle(ctx, m)
		return
	}

	storageKey := BuildStorageKey(w.consumer, key)
	acquired, state, err := w.keeper.AttemptLockWithOpts(ctx, storageKey, w.opts.lockOpts)
	if err != nil {
		w.logger.ErrorContext(ctx, "idempotency: failed to acquire lock",
			slog.String("consumer", w.consumer),
			slog.String("key", storageKey),
			slog.String("error", err.Error()),
		)
		w.metrics.inc(outcomeStorageError)
		_ = m.NakWithBackOff(w.opts.errorBackOff)
		return
	}

	if !acquired {
		if state != nil && state.Status == idempotency.StatusSuccess {
			w.metrics.inc(outcomeDuplicate)
			_ = m.Ack()
			return
		}
		w.metrics.inc(outcomeBusy)
		_ = m.NakWithBackOff(w.opts.busyBackOff)
		return
	}

	dec := &idempotencyAcker{
		orig:       m,
		keeper:     w.keeper,
		storageKey: storageKey,
		state:      state,
		consumer:   w.consumer,
		metrics:    w.metrics,
		logger:     w.logger,
		ctx:        ctx,
	}

	cp := *m
	msg.WithAcker(dec)(&cp)

	defer func() { //nolint:contextcheck // release derives its context from the acker's ctx via WithoutCancel
		if r := recover(); r != nil {
			dec.releaseOnPanic()
			panic(r)
		}
	}()

	w.handler.Handle(ctx, &cp)
}

type idempotencyAcker struct {
	orig       *msg.Message
	keeper     *idempotency.Keeper
	storageKey string
	state      *idempotency.State
	consumer   string
	metrics    *brokerMetrics
	logger     *slog.Logger
	ctx        context.Context

	once sync.Once
}

func (a *idempotencyAcker) Ack() error {
	a.once.Do(func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), defaultReleaseTimeout)
		defer cancel()

		err := a.keeper.Complete(releaseCtx, a.storageKey, nil, a.state)
		if err != nil {
			if errors.Is(err, idempotency.ErrLockStolen) {
				a.metrics.inc(outcomeLockStolen)
				a.logger.WarnContext(releaseCtx, "idempotency: lock was stolen during processing",
					slog.String("consumer", a.consumer),
					slog.String("key", a.storageKey),
				)
			} else {
				a.metrics.inc(outcomeStorageError)
				a.logger.ErrorContext(releaseCtx, "idempotency: complete failed",
					slog.String("consumer", a.consumer),
					slog.String("key", a.storageKey),
					slog.String("error", err.Error()),
				)
			}
		} else {
			a.metrics.inc(outcomeProcessed)
		}
	})
	return a.orig.Ack()
}

func (a *idempotencyAcker) Nak(delay ...time.Duration) error {
	a.once.Do(func() {
		a.release("Nak")
	})
	return a.orig.Nak(delay...)
}

func (a *idempotencyAcker) NakWithBackOff(backOff msg.BackOffFunc) error {
	a.once.Do(func() {
		a.release("NakWithBackOff")
	})
	return a.orig.NakWithBackOff(backOff)
}

func (a *idempotencyAcker) Term(reason ...string) error {
	a.once.Do(func() {
		a.release("Term")
	})
	return a.orig.Term(reason...)
}

func (a *idempotencyAcker) InProgress() error {
	return a.orig.InProgress()
}

func (a *idempotencyAcker) releaseOnPanic() {
	a.once.Do(func() {
		a.release("panic")
	})
}

func (a *idempotencyAcker) release(trigger string) {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), defaultReleaseTimeout)
	defer cancel()

	if err := a.keeper.Release(releaseCtx, a.storageKey, a.state); err != nil {
		a.logger.ErrorContext(releaseCtx, "idempotency: release failed",
			slog.String("consumer", a.consumer),
			slog.String("key", a.storageKey),
			slog.String("trigger", trigger),
			slog.String("error", err.Error()),
		)
	}
}
