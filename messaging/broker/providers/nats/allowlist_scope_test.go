// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

type noopHandler struct{ topic string }

func (noopHandler) Handle(context.Context, *msg.Message) {}
func (h noopHandler) Topic() string                      { return h.topic }

// TestSubscribe_AllowlistRejectsWiderSubscription pins subset semantics: a
// subscription is allowed only when every subject it can deliver is covered
// by an allowlist pattern.
func TestSubscribe_AllowlistRejectsWiderSubscription(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, js := natstest.ConnectJetStream(t, ns)
	_, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)

	tests := []struct {
		name      string
		allowlist []string
		topic     string
		allowed   bool
	}{
		{"star pattern rejects tail wildcard", []string{"orders.*"}, "orders.>", false},
		{"catch-all star rejects tail wildcard", []string{"*"}, ">", false},
		{"tail wildcard covers tail wildcard", []string{"orders.>"}, "orders.>", true},
		{"tail wildcard covers star", []string{"orders.>"}, "orders.*", true},
		{"star pattern covers literal", []string{"orders.*"}, "orders.created", true},
		{"star pattern covers star", []string{"orders.*"}, "orders.*", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider, err := natsprovider.New(conn, natsprovider.WithAllowedSubjects(tc.allowlist...))
			require.NoError(t, err)
			sub := natsprovider.SubscriberWithEphemeralConsumer()(provider)
			t.Cleanup(sub.Unsubscribe)
			err = sub.Subscribe(t.Context(), noopHandler{topic: tc.topic})
			if tc.allowed {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, natsprovider.ErrSubjectNotAllowed)
		})
	}
}

// TestSubscribe_ConsumerScopedToAllowlist verifies that a consumer reading
// subjects outside the allowlist is refused before it is created or updated.
func TestSubscribe_ConsumerScopedToAllowlist(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, js := natstest.ConnectJetStream(t, ns)
	_, err := js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name: "MULTI", Subjects: []string{"orders.>", "secrets.>"},
	})
	require.NoError(t, err)
	provider, err := natsprovider.New(conn, natsprovider.WithAllowedSubjects("orders.>"))
	require.NoError(t, err)

	filterOf := func(t *testing.T, name string) string {
		t.Helper()
		c, err := js.Consumer(t.Context(), "MULTI", name)
		require.NoError(t, err)
		return c.CachedInfo().Config.FilterSubject
	}

	t.Run("custom config with a broader filter is refused and does not update the consumer", func(t *testing.T) {
		t.Parallel()
		_, err := js.CreateOrUpdateConsumer(t.Context(), "MULTI", jetstream.ConsumerConfig{
			Durable: "custom-existing", FilterSubject: "orders.created", AckPolicy: jetstream.AckExplicitPolicy,
		})
		require.NoError(t, err)

		sub := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
			Durable: "custom-existing", FilterSubject: "secrets.>", AckPolicy: jetstream.AckExplicitPolicy,
		})(provider)
		t.Cleanup(sub.Unsubscribe)
		err = sub.Subscribe(t.Context(), noopHandler{topic: "orders.created"})
		require.ErrorIs(t, err, natsprovider.ErrSubjectNotAllowed)
		require.Equal(t, "orders.created", filterOf(t, "custom-existing"))
	})

	t.Run("custom config with no filter is refused and creates nothing", func(t *testing.T) {
		t.Parallel()
		sub := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
			Durable: "custom-nofilter", AckPolicy: jetstream.AckExplicitPolicy,
		})(provider)
		t.Cleanup(sub.Unsubscribe)
		err := sub.Subscribe(t.Context(), noopHandler{topic: "orders.created"})
		require.ErrorIs(t, err, natsprovider.ErrSubjectNotAllowed)
		_, err = js.Consumer(t.Context(), "MULTI", "custom-nofilter")
		require.ErrorIs(t, err, jetstream.ErrConsumerNotFound)
	})

	t.Run("an existing durable broader than the allowlist is never rewritten", func(t *testing.T) {
		t.Parallel()
		_, err := js.CreateOrUpdateConsumer(t.Context(), "MULTI", jetstream.ConsumerConfig{
			Durable: "shared-wide", FilterSubject: "secrets.>", AckPolicy: jetstream.AckExplicitPolicy,
		})
		require.NoError(t, err)

		sub := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
			Durable: "shared-wide", FilterSubject: "orders.created", AckPolicy: jetstream.AckExplicitPolicy,
		})(provider)
		t.Cleanup(sub.Unsubscribe)
		err = sub.Subscribe(t.Context(), noopHandler{topic: "orders.created"})
		require.ErrorIs(t, err, natsprovider.ErrSubjectNotAllowed)
		require.Equal(t, "secrets.>", filterOf(t, "shared-wide"))
	})

	t.Run("named consumer with a filter outside the allowlist is refused", func(t *testing.T) {
		t.Parallel()
		_, err := js.CreateOrUpdateConsumer(t.Context(), "MULTI", jetstream.ConsumerConfig{
			Durable: "named-secrets", FilterSubject: "secrets.>", AckPolicy: jetstream.AckExplicitPolicy,
		})
		require.NoError(t, err)

		sub := natsprovider.SubscriberWithConsumerName("named-secrets")(provider)
		t.Cleanup(sub.Unsubscribe)
		err = sub.Subscribe(t.Context(), noopHandler{topic: "orders.created"})
		require.ErrorIs(t, err, natsprovider.ErrSubjectNotAllowed)
		require.Equal(t, "secrets.>", filterOf(t, "named-secrets"))
	})

	t.Run("named consumer without a filter is whole-stream access and is refused", func(t *testing.T) {
		t.Parallel()
		_, err := js.CreateOrUpdateConsumer(t.Context(), "MULTI", jetstream.ConsumerConfig{
			Durable: "named-all", AckPolicy: jetstream.AckExplicitPolicy,
		})
		require.NoError(t, err)

		sub := natsprovider.SubscriberWithConsumerName("named-all")(provider)
		t.Cleanup(sub.Unsubscribe)
		err = sub.Subscribe(t.Context(), noopHandler{topic: "orders.created"})
		require.ErrorIs(t, err, natsprovider.ErrSubjectNotAllowed)
	})

	t.Run("named consumer within the allowlist is accepted", func(t *testing.T) {
		t.Parallel()
		_, err := js.CreateOrUpdateConsumer(t.Context(), "MULTI", jetstream.ConsumerConfig{
			Durable: "named-orders", FilterSubject: "orders.created", AckPolicy: jetstream.AckExplicitPolicy,
		})
		require.NoError(t, err)

		sub := natsprovider.SubscriberWithConsumerName("named-orders")(provider)
		t.Cleanup(sub.Unsubscribe)
		require.NoError(t, sub.Subscribe(t.Context(), noopHandler{topic: "orders.created"}))
	})
}

// TestSubscribe_UnfilteredConsumerFailsClosed verifies that a consumer without
// filters is accepted only when the stream's whole scope is provably inside
// the topic and the allowlist.
func TestSubscribe_UnfilteredConsumerFailsClosed(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, js := natstest.ConnectJetStream(t, ns)
	ctx := t.Context()
	for _, cfg := range []jetstream.StreamConfig{
		{Name: "SECRETS", Subjects: []string{"secrets.>"}},
		{Name: "SOURCED", Subjects: []string{"sourced.>"}, Sources: []*jetstream.StreamSource{{Name: "SECRETS"}}},
		{Name: "WIDE", Subjects: []string{"wide.>"}},
		{Name: "EXACT", Subjects: []string{"exact.one"}},
	} {
		_, err := js.CreateStream(ctx, cfg)
		require.NoError(t, err)
	}
	for stream := range map[string]struct{}{"SOURCED": {}, "WIDE": {}, "EXACT": {}} {
		_, err := js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{Durable: "all", AckPolicy: jetstream.AckExplicitPolicy})
		require.NoError(t, err)
	}
	provider, err := natsprovider.New(conn, natsprovider.WithAllowedSubjects("sourced.>", "wide.>", "exact.one"))
	require.NoError(t, err)

	tests := []struct {
		name    string
		topic   string
		allowed bool
	}{
		{"sourced stream is refused", "sourced.one", false},
		{"topic narrower than the stream is refused", "wide.one", false},
		{"stream exactly the topic is accepted", "exact.one", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := natsprovider.SubscriberWithConsumerName("all")(provider)
			t.Cleanup(sub.Unsubscribe)
			err := sub.Subscribe(t.Context(), noopHandler{topic: tc.topic})
			if tc.allowed {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, natsprovider.ErrSubjectNotAllowed)
		})
	}
}

// TestNew_RejectsMalformedAllowlistPatterns verifies that a pattern the
// matchers would read more broadly than written is a construction error.
func TestNew_RejectsMalformedAllowlistPatterns(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, _ := natstest.ConnectJetStream(t, ns)

	for _, pattern := range []string{"orders.>.created", ">.orders", "ord*.x", "orders..x", "", "orders. x", "a.>b"} {
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()
			_, err := natsprovider.New(conn, natsprovider.WithAllowedSubjects("orders.created", pattern))
			require.ErrorIs(t, err, natsprovider.ErrInvalidAllowedSubject)
		})
	}
	_, err := natsprovider.New(conn, natsprovider.WithAllowedSubjects("orders.*", "events.>", ">"))
	require.NoError(t, err)
}
