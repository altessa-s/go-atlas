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

	"github.com/nats-io/nats.go"

	"github.com/altessa-s/go-atlas/data/outbox"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Store is an alias for the generic outbox.Store interface.
type Store = outbox.Store

// Event is an alias for the generic outbox.Event type.
type Event = outbox.Event

// Status is an alias for the generic outbox.Status type.
type Status = outbox.Status

// Stats is an alias for the generic outbox.Stats snapshot.
type Stats = outbox.Stats

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
// with [msg.Message] conversion and NATS-specific retry logic.
// It implements the [broker.Outboxer] interface.
type Outbox struct {
	*outbox.Outbox
}

// brokerPayload is the serialization envelope for broker messages stored in the outbox.
type brokerPayload struct {
	Data     []byte            `json:"d"`
	Metadata map[string]string `json:"m,omitempty"`
}

// New creates a new broker-specific Outbox adapter.
// It wraps the generic outbox with msg.Message→Event conversion and NATS error handling.
func New(store Store, publisher Publisher, opts ...Option) *Outbox {
	handler := func(ctx context.Context, event outbox.Event) error {
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

	genericOpts := convertOptions(opts...)
	genericOpts = append(genericOpts, outbox.WithShouldRetry(func(err error) bool {
		if errors.Is(err, ErrUndeliverable) {
			return false
		}
		return !errors.Is(err, nats.ErrConnectionClosed)
	}))

	return &Outbox{Outbox: outbox.New(store, handler, genericOpts...)}
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

	payload, _ := json.Marshal(brokerPayload{ //nolint:errcheck // brokerPayload contains only []byte and map[string]string, always marshalable
		Data:     m.Data,
		Metadata: m.Metadata.Map(),
	})

	return outbox.Event{
		Key:       m.Topic,
		Payload:   payload,
		CreatedAt: createdTime,
	}
}
