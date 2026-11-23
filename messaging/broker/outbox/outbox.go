// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrUndeliverable marks a failure that another attempt cannot fix — a stored
// payload this adapter cannot decode. The outbox dead-letters such an event
// immediately rather than spending its whole retry budget on it.
var ErrUndeliverable = errors.New("broker outbox: event is permanently undeliverable")

// Publisher defines the interface for message transmission to external brokers.
// The [Outbox] delegates final publishing to implementations of this interface.
// Implementations must be safe for concurrent use.
type Publisher interface {
	// Publish sends a message to the target broker system.
	// Errors are handled by the [Outbox] retry logic.
	Publish(ctx context.Context, msg msg.Message) error
}

// Outbox is a broker-specific adapter that wraps the generic [outbox.Outbox]
// with [msg.Message] conversion and the broker retry classification.
// It implements the [broker.Outboxer] interface.
type Outbox struct {
	*outbox.Outbox
}

// brokerPayload is the serialization envelope for broker messages stored in the outbox.
type brokerPayload struct {
	Data     []byte            `json:"d"`
	Metadata map[string]string `json:"m,omitempty"`
}

// New creates a new broker-specific Outbox adapter. It wraps the generic
// outbox with the [NewHandler] msg.Message conversion and the [ShouldRetry]
// classification. opts configure the underlying [outbox.Outbox]; an
// [outbox.WithShouldRetry] among them is overridden by [ShouldRetry].
func New(store outbox.Storage, publisher Publisher, opts ...outbox.Option) *Outbox {
	opts = append(slices.Clip(opts), outbox.WithShouldRetry(ShouldRetry))
	return &Outbox{Outbox: outbox.New(store, NewHandler(publisher), opts...)}
}

// NewHandler returns the [outbox.Handler] that decodes a stored broker
// envelope and publishes it through publisher as a [msg.Message]. Pair it
// with [ShouldRetry] when assembling a generic outbox that delivers broker
// messages, as [New] and the broker factory do.
func NewHandler(publisher Publisher) outbox.Handler {
	return func(ctx context.Context, event outbox.Event) error {
		var bp brokerPayload
		if err := json.Unmarshal(event.Payload, &bp); err != nil {
			// A payload this adapter cannot decode will not decode on the
			// next attempt either. Wrapping it in ErrUndeliverable tells the
			// outbox to dead-letter it now instead of retrying ten times.
			return coreerrs.WrapOperation(errors.Join(ErrUndeliverable, err), "unmarshal broker payload")
		}

		meta := msg.MetaFromMap(bp.Metadata)
		// The outbox publishes at-least-once by construction, so the broker
		// needs a stable identity to collapse the repeats. Event.Id is exactly
		// that: assigned once at Save and unchanged across every retry. A
		// caller-supplied deduplication ID wins — it is usually derived from
		// the business entity, which dedupes across producers too, not just
		// across this event's own retries.
		if did, found := meta.Value(msg.MetaKeyDeduplicateId); !found || did == "" {
			meta = append(meta, msg.MetaData{Key: msg.MetaKeyDeduplicateId, Value: event.Id})
		}

		return publisher.Publish(ctx, msg.Message{
			Metadata: meta,
			Data:     bp.Data,
			Topic:    event.Key,
		})
	}
}

// ShouldRetry is the broker retry predicate for [outbox.WithShouldRetry]:
// [ErrUndeliverable] and [broker.ErrConnectionClosed] are permanent, every
// other error is transient. Providers mark their driver's closed-connection
// error with [broker.ErrConnectionClosed]; a raw driver error is transient.
func ShouldRetry(err error) bool {
	return !errors.Is(err, ErrUndeliverable) && !errors.Is(err, broker.ErrConnectionClosed)
}

// Publish saves a single message to the outbox store for later publishing.
// Should be called within the same transaction as the business logic for atomicity.
func (o *Outbox) Publish(ctx context.Context, m msg.Message) error {
	return o.Save(ctx, msgToEvent(m))
}

// PublishBatch saves multiple messages to the outbox store in a single operation.
// Returns nil immediately if no messages are provided.
func (o *Outbox) PublishBatch(ctx context.Context, msgs ...msg.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	events := slices.Collect(coreslices.Map(msgs, msgToEvent))
	return o.Save(ctx, events...)
}

// msgToEvent converts a msg.Message to an outbox.Event.
// Broker-specific data (Data + Metadata) is serialized into the generic Payload field.
func msgToEvent(m msg.Message) outbox.Event {
	createdTime := time.Now().UTC()
	if created, err := msg.MessageCreatedTimeFromMeta(m.Metadata); err == nil && !created.IsZero() {
		createdTime = created
	}

	metadata := m.Metadata.Map()
	orderKey := metadata[msg.MetaKeyOrderKey]
	delete(metadata, msg.MetaKeyOrderKey)
	payload, _ := json.Marshal(brokerPayload{ //nolint:errcheck // brokerPayload contains only []byte and map[string]string, always marshalable
		Data:     m.Data,
		Metadata: metadata,
	})

	return outbox.Event{
		Key:       m.Topic,
		OrderKey:  orderKey,
		Payload:   payload,
		CreatedAt: createdTime,
	}
}
