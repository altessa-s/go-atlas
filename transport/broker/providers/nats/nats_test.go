// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/broker"
)

type fakeSubscriber struct {
	unsubscribed int
}

func (f *fakeSubscriber) Subscribe(_ context.Context, _ broker.SubscriberHandler) error { return nil }
func (f *fakeSubscriber) Unsubscribe()                                                  { f.unsubscribed++ }
func (f *fakeSubscriber) Closed() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func TestNats_UnsubscribeAll_ClearsListAndCallsUnsubscribe(t *testing.T) {
	n := &Nats{}
	s1 := &fakeSubscriber{}
	s2 := &fakeSubscriber{}

	n.subscribers = []broker.Subscriber{s1, s2}

	n.UnsubscribeAll()

	require.Equal(t, 1, s1.unsubscribed)
	require.Equal(t, 1, s2.unsubscribed)

	// Second call should not re-unsubscribe.
	n.UnsubscribeAll()
	require.Equal(t, 1, s1.unsubscribed)
	require.Equal(t, 1, s2.unsubscribed)
}

func TestWarnIfNoAuth_LogsWarning(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(old)

	conn := &nats.Conn{} // no auth configured
	warnIfNoAuth(conn)

	require.True(t, strings.Contains(buf.String(), "connecting without authentication"), "expected warning log, got: %s", buf.String())
}

func TestWarnIfNoAuth_SilentWithToken(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(old)

	conn := &nats.Conn{}
	conn.Opts.Token = "secret"
	warnIfNoAuth(conn)

	require.Equal(t, 0, buf.Len())
}
