// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/retry"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/reliability/idempotency"

	"google.golang.org/protobuf/proto"
)

// StorageKeyPrefix is the domain tag of the storage keys managed by this package: the first component of the tuple
// [BuildStorageKey] composes, which keeps them apart from the keys of the HTTP and gRPC transports.
const StorageKeyPrefix = "ibk"

// ErrKeyTooLong terminates a message whose idempotency key is longer than the limit ([WithMaxKeyLength]), before the key
// reaches the storage.
var ErrKeyTooLong = errors.New("idempotency key too long")

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

// BuildStorageKey formats the storage key for an idempotency record: the tuple ([StorageKeyPrefix], consumer, topic,
// key) composed by [idempotency.ComposeKey], so distinct (consumer, topic, key) triples never share a storage key
// whatever bytes they hold. topic is the subject the message arrived on ([msg.Message.Topic]), which scopes a dedup id
// to the subject its publisher was allowed to publish on. The result persists in storage backends; see
// [idempotency.ComposeKey] for the frozen v2 layout.
func BuildStorageKey(consumer, topic, key string) string {
	return idempotency.ComposeKey(StorageKeyPrefix, consumer, topic, key)
}

// Keeper is the idempotency storage behavior required by [Wrap].
type Keeper interface {
	AttemptLockWithOpts(context.Context, string, idempotency.AttemptLockOpts) (bool, *idempotency.State, error)
	Complete(context.Context, string, any, *idempotency.State) error
	Release(context.Context, string, *idempotency.State) error
}

// Wrap returns a [broker.SubscriberHandler] that deduplicates incoming messages
// for the given handler h using keeper.
//
// The consumer string scopes keys so two consumers of the same topic do not collide.
// If h is nil, keeper is nil, or consumer is empty, Wrap panics.
func Wrap(h broker.SubscriberHandler, keeper Keeper, consumer string, opt ...Option) broker.SubscriberHandler {
	if h == nil {
		panic("idempotency: handler cannot be nil")
	}
	if nilcheck.IsNil(keeper) {
		panic("idempotency: keeper cannot be nil")
	}
	if consumer == "" {
		panic("idempotency: consumer cannot be empty")
	}

	options := newOptions(opt...)
	if options.maxKeyLength <= 0 {
		options.maxKeyLength = DefaultMaxKeyLength
	}
	logger := options.logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &wrappedHandler{
		handler:  h,
		keeper:   keeper,
		consumer: consumer,
		opts:     options,
		metrics:  newBrokerMetrics(options.meter, consumer),
		logger:   logger,
	}
}

// key returns the idempotency key of m: the [KeyFunc] result, refused with [ErrKeyTooLong] beyond the key length limit.
func (w *wrappedHandler) key(m *msg.Message) (string, error) {
	key, err := w.opts.keyFunc(m)
	if err != nil {
		return "", err
	}
	if len(key) > w.opts.maxKeyLength {
		return "", fmt.Errorf("%w: %d bytes, limit %d", ErrKeyTooLong, len(key), w.opts.maxKeyLength)
	}
	return key, nil
}

type wrappedHandler struct {
	handler  broker.SubscriberHandler
	keeper   Keeper
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

	key, err := w.key(m)
	if err != nil {
		w.logger.ErrorContext(ctx, "idempotency: key extraction failed",
			slog.String("consumer", w.consumer),
			slog.String("topic", w.handler.Topic()),
			slog.String("error_kind", errorKind(err)),
		)
		w.metrics.inc(outcomeKeyError)
		reason := "key extraction failed"
		if errors.Is(err, ErrKeyTooLong) {
			reason = ErrKeyTooLong.Error()
		}
		_ = m.Term("idempotency: " + reason)
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

	storageKey := BuildStorageKey(w.consumer, m.Topic, key)
	acquired, state, err := w.keeper.AttemptLockWithOpts(ctx, storageKey, w.opts.lockOpts)
	if err != nil {
		w.logger.ErrorContext(ctx, "idempotency: failed to acquire lock",
			slog.String("consumer", w.consumer),
			slog.String("error_kind", errorKind(err)),
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

	cp := m.CloneWithAcker(dec)

	defer func() { //nolint:contextcheck // release derives its context from the acker's ctx via WithoutCancel
		if r := recover(); r != nil {
			dec.releaseOnPanic()
			panic(r)
		}
	}()

	w.handler.Handle(ctx, cp)
}

type idempotencyAcker struct {
	orig       *msg.Message
	keeper     Keeper
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

		// Complete is retried within the release timeout: the effect is
		// already applied, and a record left IN_PROGRESS would let a
		// duplicate re-run the handler once MaxLockDuration passes.
		err := retry.Do(releaseCtx, func(ctx context.Context) error {
			return a.keeper.Complete(ctx, a.storageKey, nil, a.state)
		},
			retry.WithMaxAttempts(completeMaxRetries),
			retry.WithNextDelay(retry.Exponential(completeRetryBase, retry.WithExponentialMaxDelay(completeRetryMax))),
			retry.WithShouldRetry(func(err error) bool { return !errors.Is(err, idempotency.ErrLockStolen) }),
		)
		switch {
		case err == nil:
			a.metrics.inc(outcomeProcessed)
		case errors.Is(err, idempotency.ErrLockStolen):
			a.metrics.inc(outcomeLockStolen)
			a.logger.WarnContext(releaseCtx, "idempotency: lock was stolen during processing",
				slog.String("consumer", a.consumer),
			)
		default:
			// The message is still acked: the effect is applied, and a Nak
			// would only redeliver it to a busy lock.
			a.metrics.inc(outcomeCompleteFailed)
			a.logger.ErrorContext(releaseCtx, "idempotency: complete failed; the record stays in progress until the lock expires",
				slog.String("consumer", a.consumer),
				slog.String("error_kind", errorKind(err)),
			)
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
			slog.String("trigger", trigger),
			slog.String("error_kind", errorKind(err)),
		)
	}
}

// errorKind classifies err for logs without its text: keeper and key-extractor
// errors may echo the storage key or the message content, which carry the
// original business key.
func errorKind(err error) string {
	switch {
	case errors.Is(err, ErrKeyTooLong):
		return "key_too_long"
	case errors.Is(err, idempotency.ErrLockStolen):
		return "lock_stolen"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "error"
	}
}
