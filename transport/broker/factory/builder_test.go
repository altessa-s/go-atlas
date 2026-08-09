// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// White-box: the config→option conversion these tests pin is only reachable
// through createOutboxWithStore. The public entry points require a live
// MongoDB, which would turn a wiring test into an integration test.
package factory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	dataoutbox "github.com/altessa-s/go-atlas/data/outbox"
)

// capturingRegistrar records the scheduler tasks the builder registers.
type capturingRegistrar struct {
	mu         sync.Mutex
	registered []corescheduler.TaskConfig
}

func (r *capturingRegistrar) Register(_ context.Context, cfg corescheduler.TaskConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registered = append(r.registered, cfg)
	return nil
}

func (r *capturingRegistrar) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.registered))
	for _, c := range r.registered {
		out = append(out, c.ID)
	}
	return out
}

// nopStore is an outbox.Store that does nothing; these tests only care about
// how the builder was configured, never about persistence.
type nopStore struct{}

func (nopStore) FetchUnprocessedEvents(context.Context, uint32) ([]dataoutbox.Event, error) {
	return nil, nil
}
func (nopStore) DeleteProcessedEvents(context.Context, time.Duration) error { return nil }
func (nopStore) UnlockStuckEvents(context.Context, time.Duration) error     { return nil }
func (nopStore) SaveEvents(context.Context, ...dataoutbox.Event) error      { return nil }
func (nopStore) UpdateEvents(context.Context, ...dataoutbox.Event) error    { return nil }
func (nopStore) ExpireEvents(context.Context) (int64, error)                { return 0, nil }
func (nopStore) Stats(context.Context) (dataoutbox.Stats, error)            { return dataoutbox.Stats{}, nil }

// nopPublisher satisfies the outbox Publisher interface.
type nopPublisher struct{}

func (nopPublisher) Publish(context.Context, msg.Message) error { return nil }

// outboxConfig returns a fully populated broker config, so a field the builder
// forgets to read shows up as a default rather than the configured value.
func outboxConfig() *config.Broker {
	return &config.Broker{
		Outbox: config.Outbox{
			Enabled:                 true,
			FetchTimeout:            time.Second,
			HandleTimeout:           2 * time.Second,
			UpdateTimeout:           time.Second,
			MaxLockTime:             time.Minute,
			MessagesBatchSize:       10,
			RetryMaxAttempts:        3,
			RetryBaseDelay:          time.Second,
			RetryMaxDelay:           time.Minute,
			MaxPayloadBytes:         32,
			PublishedEventsLifetime: time.Hour,
			DefaultEventTTL:         time.Hour,
			DispatchSchedule:        "@every 2s",
			UnlockSchedule:          "@every 11s",
			CleanupSchedule:         "@every 10m",
			ExpireSchedule:          "@every 11s",
			StatsSchedule:           "@every 30s",
			DispatchTaskID:          "svc-a-dispatch",
			UnlockTaskID:            "svc-a-unlock",
			ExpireTaskID:            "svc-a-expire",
			CleanupTaskID:           "svc-a-cleanup",
			StatsTaskID:             "svc-a-stats",
		},
	}
}

// The builder and data/outbox/factory read the same config.Outbox. A field one
// of them forgets is not a compile error and not a runtime error — the setting
// is simply ignored, and the operator's YAML quietly does nothing.
func TestCreateOutbox_RegistersEveryCycleUnderItsConfiguredID(t *testing.T) {
	t.Parallel()

	reg := &capturingRegistrar{}
	ob, err := New(outboxConfig()).
		UseScheduler(reg).
		createOutboxWithStore(nopStore{}, nopPublisher{})
	require.NoError(t, err)
	require.NotNil(t, ob)

	require.ElementsMatch(t,
		[]string{"svc-a-dispatch", "svc-a-unlock", "svc-a-expire", "svc-a-cleanup", "svc-a-stats"},
		reg.ids(),
		"every configured cycle must reach the scheduler under its configured ID")
}

// MaxPayloadBytes is the one outbox setting whose effect is observable without a
// store, so it stands in for the whole config→option conversion.
func TestCreateOutbox_AppliesThePayloadLimitFromConfig(t *testing.T) {
	t.Parallel()

	ob, err := New(outboxConfig()).createOutboxWithStore(nopStore{}, nopPublisher{})
	require.NoError(t, err)

	oversized := make([]byte, 64) // The config allows 32.
	require.ErrorIs(t,
		ob.Publish(t.Context(), *msg.NewMessage("billing.invoice.paid", oversized)),
		dataoutbox.ErrPayloadTooLarge)
}

// A disabled outbox builds into nothing rather than into a running one.
func TestCreateOutbox_DisabledReturnsNothing(t *testing.T) {
	t.Parallel()

	cfg := outboxConfig()
	cfg.Outbox.Enabled = false

	ob, err := New(cfg).createOutboxWithStore(nopStore{}, nopPublisher{})
	require.NoError(t, err)
	require.Nil(t, ob)
}

// A missing publisher is a dependency error, not a nil outbox that fails later.
func TestCreateOutbox_RequiresAPublisher(t *testing.T) {
	t.Parallel()

	_, err := New(outboxConfig()).createOutboxWithStore(nopStore{}, nil)
	require.Error(t, err)
}
