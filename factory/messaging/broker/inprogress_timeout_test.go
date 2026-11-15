// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/inprogress"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
)

// timeoutRecorder is a slog.Handler that keeps the "timeout" attribute of
// every record, which the manager logs when it abandons an InProgress call.
type timeoutRecorder struct {
	mu       sync.Mutex
	timeouts []time.Duration
}

func (h *timeoutRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (h *timeoutRecorder) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *timeoutRecorder) WithGroup(string) slog.Handler            { return h }

func (h *timeoutRecorder) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "timeout" {
			h.timeouts = append(h.timeouts, a.Value.Duration())
		}
		return true
	})
	return nil
}

func (h *timeoutRecorder) seen() []time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]time.Duration(nil), h.timeouts...)
}

// stuckHeartbeater blocks in InProgress until release is closed.
type stuckHeartbeater struct{ release <-chan struct{} }

func (h stuckHeartbeater) InProgress() error {
	<-h.release
	return nil
}

// A heartbeater that never answers is abandoned after inProgress.heartbeatTimeout
// rather than after the manager default.
func TestCreateInProgressManager_AppliesTheHeartbeatTimeoutFromConfig(t *testing.T) {
	t.Parallel()

	const timeout = 20 * time.Millisecond
	logs := &timeoutRecorder{}
	cfg := &brokerconfig.Config{InProgress: brokerconfig.InProgress{Enabled: true, HeartbeatTimeout: timeout}}
	mgr, err := factory.New(cfg).UseLogger(slog.New(logs)).CreateInProgressManager()
	require.NoError(t, err)

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	t.Cleanup(mgr.Register(stuckHeartbeater{release: release}, 0))

	require.NoError(t, mgr.RunTickCycle(t.Context()))
	require.Equal(t, []time.Duration{timeout}, logs.seen())
}

func TestDefaultConfig_MirrorsTheManagerHeartbeatTimeout(t *testing.T) {
	t.Parallel()

	require.Equal(t, inprogress.DefaultHeartbeatTimeout, brokerconfig.Default().InProgress.HeartbeatTimeout)
}
