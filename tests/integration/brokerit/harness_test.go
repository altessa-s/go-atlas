// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerit_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	natsprovider "github.com/altessa-s/go-atlas/transport/broker/providers/nats"
)

const (
	// settleWindow is how long an assertion allows a condition to become true.
	// Generous: several redeliveries of a one-second ack wait fit inside it.
	settleWindow = 15 * time.Second

	// samplingInterval is the poll cadence for those assertions.
	samplingInterval = 20 * time.Millisecond

	// ackWait is the ack timer of the scenarios that wait for the server to
	// redeliver an unacknowledged message: long enough that a local round
	// trip never trips it, short enough to wait out several times.
	ackWait = time.Second
)

// natsURL returns the broker address, matching tests/integration/docker-compose.yml.
func natsURL() string {
	if v := os.Getenv("NATS_URL"); v != "" {
		return v
	}
	return "nats://127.0.0.1:14222"
}

// nameSequence disambiguates names generated within one clock tick by
// parallel tests.
var nameSequence atomic.Int64

// uniqueName derives a stream, consumer or bucket name from the test name.
// The test name makes leftovers traceable; the sequence and the timestamp
// keep two runs against one server — or two tests in one run — apart.
func uniqueName(tb testing.TB) string {
	tb.Helper()

	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, tb.Name())

	const maxNameLen = 40
	if len(safe) > maxNameLen {
		safe = safe[:maxNameLen]
	}

	return fmt.Sprintf("brokerit_%s_%d_%s", safe, nameSequence.Add(1), strconv.FormatInt(time.Now().UnixNano(), 36))
}

// connect dials the broker on a connection of the test's own, closed on
// cleanup. It skips when the broker is unreachable, so a machine without the
// compose stack still gets a green build.
func connect(tb testing.TB) *nats.Conn {
	tb.Helper()

	nc, err := nats.Connect(natsURL(), nats.Timeout(2*time.Second), nats.RetryOnFailedConnect(false))
	if err != nil {
		tb.Skipf("NATS unreachable at %s (%v) — start it with: docker compose -f tests/integration/docker-compose.yml up -d --wait nats", natsURL(), err)
	}
	tb.Cleanup(nc.Close)

	return nc
}

// newProvider builds a NATS provider over a fresh connection.
func newProvider(tb testing.TB) *natsprovider.NATS {
	tb.Helper()

	p, err := natsprovider.New(connect(tb))
	require.NoError(tb, err)

	return p
}

// world is one test's isolated stream: its own name, its own subject, and
// the provider that created it.
type world struct {
	provider *natsprovider.NATS
	stream   jetstream.Stream
	name     string
	subject  string
}

// newWorld creates a throwaway memory stream capturing one subject of its own,
// deleted on cleanup. JetStream refuses two streams with overlapping subjects,
// so parallel tests cannot share one.
func newWorld(tb testing.TB) *world {
	tb.Helper()

	p := newProvider(tb)
	name := uniqueName(tb)
	subject := "brokerit." + name

	stream, err := p.CreateStream(tb.Context(), &jetstream.StreamConfig{
		Name:     name,
		Subjects: []string{subject},
		Storage:  jetstream.MemoryStorage,
	})
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = p.DeleteStream(context.Background(), name) })

	return &world{provider: p, stream: stream, name: name, subject: subject}
}

// durable is the configuration of an explicitly acknowledged durable consumer
// on the world's subject — the shape a production service subscribes with.
func (w *world) durable(name string, wait time.Duration, mods ...func(*jetstream.ConsumerConfig)) *jetstream.ConsumerConfig {
	cfg := &jetstream.ConsumerConfig{
		Durable:       name,
		FilterSubject: w.subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckWait:       wait,
	}
	for _, mod := range mods {
		mod(cfg)
	}

	return cfg
}

// subscribe starts handler on provider with factory and unsubscribes it on
// cleanup.
func subscribe(tb testing.TB, p *natsprovider.NATS, factory broker.SubscriberFactory, handler broker.SubscriberHandler) broker.Subscriber {
	tb.Helper()

	sub := p.Subscriber(factory)
	require.NoError(tb, sub.Subscribe(tb.Context(), handler))
	tb.Cleanup(sub.Unsubscribe)

	return sub
}

// publish sends one message per payload, in order, through a broker over the
// world's provider.
func (w *world) publish(tb testing.TB, payloads ...string) {
	tb.Helper()

	b := broker.New(w.provider)
	for _, p := range payloads {
		require.NoError(tb, b.Publish(tb.Context(), *msg.NewMessage(w.subject, []byte(p))))
	}
}

// payloads returns n distinct payloads with the given prefix.
func payloads(prefix string, n int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = fmt.Sprintf("%s-%03d", prefix, i)
	}

	return out
}

// consumerInfo reads the server's view of a durable consumer.
func (w *world) consumerInfo(tb testing.TB, name string) *jetstream.ConsumerInfo {
	tb.Helper()

	c, err := w.stream.Consumer(tb.Context(), name)
	require.NoError(tb, err)
	info, err := c.Info(tb.Context())
	require.NoError(tb, err)

	return info
}

// awaitSettled waits until the server holds nothing pending and nothing
// awaiting an ack for the durable consumer: every message has been delivered
// and acknowledged.
func (w *world) awaitSettled(tb testing.TB, name string) *jetstream.ConsumerInfo {
	tb.Helper()

	// The condition runs on its own goroutine, so it reports a failed read as
	// "not yet" rather than failing the test from there.
	var info *jetstream.ConsumerInfo
	require.Eventually(tb, func() bool {
		c, err := w.stream.Consumer(tb.Context(), name)
		if err != nil {
			return false
		}
		got, err := c.Info(tb.Context())
		if err != nil {
			return false
		}
		info = got
		return got.NumPending == 0 && got.NumAckPending == 0
	}, settleWindow, samplingInterval, "consumer %s did not settle", name)

	return info
}
