// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerit_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/tests/integration/brokerit"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

// A stream captures two subjects and the allowlist covers one: a custom
// consumer config with a filter outside the allowlist and a named consumer
// reading the whole stream are both refused, and the existing durable
// consumer's configuration is left untouched by the refusal.
func TestAllowlist_ConsumerScopeIsEnforcedOnALiveServer(t *testing.T) {
	t.Parallel()

	name := uniqueName(t)
	allowed, other := "brokerit."+name+".orders", "brokerit."+name+".secrets"
	p := newProvider(t, natsprovider.WithAllowedSubjects(allowed))
	_, err := p.CreateStream(t.Context(), &jetstream.StreamConfig{
		Name: name, Subjects: []string{allowed, other}, Storage: jetstream.MemoryStorage,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.DeleteStream(context.WithoutCancel(t.Context()), name) })

	_, err = p.CreateConsumer(t.Context(), name, &jetstream.ConsumerConfig{
		Durable: "existing", FilterSubject: allowed, AckPolicy: jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)
	_, err = p.CreateConsumer(t.Context(), name, &jetstream.ConsumerConfig{
		Durable: "whole", AckPolicy: jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	handler := brokerit.NewRecorder(allowed, func(context.Context, *msg.Message, int) {})

	widened := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
		Durable: "existing", FilterSubject: other, AckPolicy: jetstream.AckExplicitPolicy,
	})(p)
	require.ErrorIs(t, widened.Subscribe(t.Context(), handler), natsprovider.ErrSubjectNotAllowed)

	whole := natsprovider.SubscriberWithConsumerName("whole")(p)
	require.ErrorIs(t, whole.Subscribe(t.Context(), handler), natsprovider.ErrSubjectNotAllowed)

	consumer, err := p.JetStream().Consumer(t.Context(), name, "existing")
	require.NoError(t, err)
	require.Equal(t, allowed, consumer.CachedInfo().Config.FilterSubject)
}
