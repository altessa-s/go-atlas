// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/observability/health"

	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// recordHandler is a slog handler that keeps every record's message and level.
type recordHandler struct {
	mu   sync.Mutex
	msgs []string
	lvls []slog.Level
}

func (h *recordHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *recordHandler) WithGroup(string) slog.Handler            { return h }

func (h *recordHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = append(h.msgs, r.Message)
	h.lvls = append(h.lvls, r.Level)
	return nil
}

func (h *recordHandler) last() (string, slog.Level) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.msgs[len(h.msgs)-1], h.lvls[len(h.lvls)-1]
}

func TestBuild_NilConfig(t *testing.T) {
	t.Parallel()
	_, err := New(nil).Build()
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

func TestBuild_ConnectsAndReportsHealth(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	coord := health.New()
	t.Cleanup(coord.Close)

	conn, err := New(&natsconfig.Config{
		Hosts:          []string{ns.ClientURL()},
		ClientName:     "cov",
		ConnectTimeout: time.Second,
		ReconnectWait:  10 * time.Millisecond,
		MaxReconnect:   1,
	}).UseDefaultLogger().UseLogger(slog.New(slog.DiscardHandler)).UseHealthCoordinator(coord).Build()
	require.NoError(t, err)
	t.Cleanup(conn.Close)

	require.True(t, conn.IsConnected())
	require.Equal(t, health.StatusServing, coord.CheckServiceHealth(t.Context(), "nats"))

	conn.Close()
	require.Equal(t, health.StatusNotServing, coord.CheckServiceHealth(t.Context(), "nats"))
}

// RetryOnFailedConnect keeps an unreachable server from failing Build; the
// connection keeps reconnecting and health reports it as degraded.
func TestBuild_UnreachableServerIsDegraded(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	conn, err := New(&natsconfig.Config{
		ConnectionURI:  redacted.RedactedString("nats://127.0.0.1:1"),
		ConnectTimeout: 100 * time.Millisecond,
		ReconnectWait:  time.Hour,
	}).UseLogger(slog.New(slog.DiscardHandler)).UseHealthCoordinator(coord).UseHealthServiceName("bus").Build()
	require.NoError(t, err)
	t.Cleanup(conn.Close)

	require.False(t, conn.IsConnected())
	require.Equal(t, health.StatusDegraded, coord.CheckServiceHealth(t.Context(), "bus"))
}

func TestBuild_UnparsableURLFails(t *testing.T) {
	t.Parallel()
	_, err := New(&natsconfig.Config{
		ConnectionURI: redacted.RedactedString("nats://[::1"),
	}).Build()
	require.ErrorContains(t, err, "connect to NATS at")
}

func TestBuild_UnparsableURLDoesNotLeakCredentials(t *testing.T) {
	t.Parallel()
	_, err := New(&natsconfig.Config{
		ConnectionURI: redacted.RedactedString("nats://user:s3cr3t@[::1"),
	}).Build()
	require.ErrorContains(t, err, "connect to NATS at")
	require.NotContains(t, err.Error(), "s3cr3t")
	require.NotContains(t, err.Error(), "user:")
}

func TestConnectionHandlersLog(t *testing.T) {
	t.Parallel()
	h := &recordHandler{}
	logger := slog.New(h)

	natsDisconnectErrHandler(logger)(nil, errors.New("eof"))
	msg, lvl := h.last()
	require.Equal(t, "NATS connection lost", msg)
	require.Equal(t, slog.LevelWarn, lvl)

	natsDisconnectErrHandler(logger)(nil, nil)
	msg, lvl = h.last()
	require.Equal(t, "NATS connection disconnected", msg)
	require.Equal(t, slog.LevelInfo, lvl)

	natsErrorHandler(logger)(nil, &nats.Subscription{Subject: "orders"}, errors.New("slow consumer"))
	msg, _ = h.last()
	require.Equal(t, "NATS subscription error", msg)

	natsErrorHandler(logger)(nil, nil, errors.New("auth"))
	msg, lvl = h.last()
	require.Equal(t, "NATS connection error", msg)
	require.Equal(t, slog.LevelError, lvl)

	ns := natstest.StartNATSServer(t)
	conn := natstest.ConnectNATS(t, ns)
	natsReconnectHandler(logger)(conn)
	msg, lvl = h.last()
	require.Equal(t, "NATS connection re-established", msg)
	require.Equal(t, slog.LevelDebug, lvl)
}

func TestBuild_BuffersPublishDuringReconnect(t *testing.T) {
	t.Parallel()

	storeDir := t.TempDir()
	startOn := func(port int) *server.Server {
		ns, err := server.NewServer(&server.Options{
			Host:      "127.0.0.1",
			Port:      port,
			NoLog:     true,
			NoSigs:    true,
			JetStream: true,
			StoreDir:  storeDir,
		})
		require.NoError(t, err)
		ns.Start()
		require.True(t, ns.ReadyForConnections(5*time.Second))
		return ns
	}

	ns1 := startOn(-1)
	stoppedFirst := false
	t.Cleanup(func() {
		if !stoppedFirst {
			ns1.Shutdown()
			ns1.WaitForShutdown()
		}
	})

	port := ns1.Addr().(*net.TCPAddr).Port
	url := ns1.ClientURL()

	_, js1 := natstest.ConnectJetStream(t, ns1)
	_, err := js1.CreateStream(t.Context(), jetstream.StreamConfig{
		Name:     "ORDERS",
		Subjects: []string{"orders.>"},
		Storage:  jetstream.FileStorage,
	})
	require.NoError(t, err)

	conn, err := New(&natsconfig.Config{
		Hosts:          []string{url},
		ClientName:     "reconnect-buf-test",
		ConnectTimeout: time.Second,
		ReconnectWait:  20 * time.Millisecond,
	}).Build()
	require.NoError(t, err)
	t.Cleanup(conn.Close)

	ns1.Shutdown()
	ns1.WaitForShutdown()
	stoppedFirst = true

	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		return !conn.IsConnected()
	}, "client disconnected")

	require.NoError(t, conn.Publish("orders.created", []byte("buffered-during-outage")))

	ns2 := startOn(port)
	t.Cleanup(func() {
		ns2.Shutdown()
		ns2.WaitForShutdown()
	})

	testhelpers.WaitFor(t, 5*time.Second, conn.IsConnected, "client reconnected")
	require.NoError(t, conn.FlushTimeout(2*time.Second))

	_, js2 := natstest.ConnectJetStream(t, ns2)
	stream, err := js2.Stream(t.Context(), "ORDERS")
	require.NoError(t, err)
	raw, err := stream.GetMsg(t.Context(), 1)
	require.NoError(t, err)
	require.Equal(t, []byte("buffered-during-outage"), raw.Data)
}
